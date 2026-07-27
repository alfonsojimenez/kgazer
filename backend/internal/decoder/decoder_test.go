package decoder

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linkedin/goavro/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// confluentFrame builds a Confluent wire-format header (magic byte + big
// endian schema ID) followed by body.
func confluentFrame(schemaID int32, body []byte) []byte {
	buf := make([]byte, 5+len(body))
	buf[0] = 0x00
	binary.BigEndian.PutUint32(buf[1:5], uint32(schemaID))
	copy(buf[5:], body)
	return buf
}

func TestReadMessageIndexes(t *testing.T) {
	t.Run("length zero shorthand", func(t *testing.T) {
		payload := []byte{0x00} // zigzag varint 0
		n, indexes, err := readMessageIndexes(payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Errorf("bytesRead = %d, want 1", n)
		}
		if len(indexes) != 1 || indexes[0] != 0 {
			t.Errorf("indexes = %v, want [0]", indexes)
		}
	})

	t.Run("explicit multi-index array", func(t *testing.T) {
		var buf []byte
		buf = binary.AppendVarint(buf, 2) // array length 2
		buf = binary.AppendVarint(buf, 1) // top-level index
		buf = binary.AppendVarint(buf, 3) // nested index
		buf = append(buf, 0xAB, 0xCD)     // trailing protobuf payload bytes

		n, indexes, err := readMessageIndexes(buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(indexes) != 2 || indexes[0] != 1 || indexes[1] != 3 {
			t.Errorf("indexes = %v, want [1 3]", indexes)
		}
		if n != len(buf)-2 {
			t.Errorf("bytesRead = %d, want %d", n, len(buf)-2)
		}
	})

	t.Run("truncated buffer errors", func(t *testing.T) {
		_, _, err := readMessageIndexes(nil)
		if err == nil {
			t.Fatal("expected error for empty payload")
		}
	})
}

const simpleProtoSchema = `
syntax = "proto3";
message Simple {
  string name = 1;
  int32 value = 2;
}
`

const nestedProtoSchema = `
syntax = "proto3";
message Outer {
  message Inner {
    string label = 1;
  }
  Inner inner = 1;
}
message AfterOuter {
  string tag = 1;
}
`

// newSchemaRegistryStub returns an httptest.Server that serves the given
// schema for any /schemas/ids/{id} request, tagged with schemaType.
func newSchemaRegistryStub(t *testing.T, schema, schemaType string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := schemaResponse{Schema: schema, SchemaType: schemaType}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveMessageDescriptor(t *testing.T) {
	srv := newSchemaRegistryStub(t, nestedProtoSchema, "PROTOBUF")
	dec := New(srv.URL, "", "")

	meta, err := dec.getSchemaMeta(1)
	if err != nil {
		t.Fatalf("getSchemaMeta: %v", err)
	}
	files, err := dec.getProtoFiles(1, meta)
	if err != nil {
		t.Fatalf("getProtoFiles: %v", err)
	}
	mainFile := files.FindFileByPath(mainProtoFile)
	if mainFile == nil {
		t.Fatal("compiled file not found")
	}

	t.Run("top-level message", func(t *testing.T) {
		md, err := resolveMessageDescriptor(mainFile.Messages(), []int{1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(md.Name()) != "AfterOuter" {
			t.Errorf("resolved message = %s, want AfterOuter", md.Name())
		}
	})

	t.Run("nested message", func(t *testing.T) {
		md, err := resolveMessageDescriptor(mainFile.Messages(), []int{0, 0})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(md.Name()) != "Inner" {
			t.Errorf("resolved message = %s, want Inner", md.Name())
		}
	})

	t.Run("out of range index errors", func(t *testing.T) {
		if _, err := resolveMessageDescriptor(mainFile.Messages(), []int{99}); err == nil {
			t.Fatal("expected out-of-range error")
		}
	})
}

func TestDecodeProtobuf(t *testing.T) {
	srv := newSchemaRegistryStub(t, simpleProtoSchema, "PROTOBUF")
	dec := New(srv.URL, "", "")

	// Build the wire payload: message index [0] (shorthand), then the raw
	// protobuf bytes for Simple{name: "hello", value: 42}, constructed via a
	// dynamicpb message built from the same compiled descriptor so the test
	// stays independent of any generated code.
	meta, err := dec.getSchemaMeta(7)
	if err != nil {
		t.Fatalf("getSchemaMeta: %v", err)
	}
	files, err := dec.getProtoFiles(7, meta)
	if err != nil {
		t.Fatalf("getProtoFiles: %v", err)
	}
	md, err := resolveMessageDescriptor(files.FindFileByPath(mainProtoFile).Messages(), []int{0})
	if err != nil {
		t.Fatalf("resolveMessageDescriptor: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	msg.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("hello"))
	msg.Set(md.Fields().ByName("value"), protoreflect.ValueOfInt32(42))
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshaling test message: %v", err)
	}

	payload := append([]byte{0x00}, body...) // message index [0] shorthand
	raw := confluentFrame(7, payload)

	jsonBytes, format, err := dec.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if format != "protobuf" {
		t.Errorf("format = %q, want protobuf", format)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("unmarshaling result: %v", err)
	}
	if got["name"] != "hello" {
		t.Errorf("name = %v, want hello", got["name"])
	}
	if got["value"] != float64(42) {
		t.Errorf("value = %v, want 42", got["value"])
	}
}

func TestDecodeAvroUnchanged(t *testing.T) {
	avroSchema := `{"type":"record","name":"Simple","fields":[{"name":"name","type":"string"},{"name":"value","type":"int"}]}`
	codec, err := goavro.NewCodec(avroSchema)
	if err != nil {
		t.Fatalf("goavro.NewCodec: %v", err)
	}
	body, err := codec.BinaryFromNative(nil, map[string]interface{}{"name": "hello", "value": 42})
	if err != nil {
		t.Fatalf("BinaryFromNative: %v", err)
	}

	t.Run("schemaType omitted defaults to avro", func(t *testing.T) {
		srv := newSchemaRegistryStub(t, avroSchema, "")
		dec := New(srv.URL, "", "")
		raw := confluentFrame(1, body)

		jsonBytes, format, err := dec.Decode(raw)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if format != "avro" {
			t.Errorf("format = %q, want avro", format)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &got); err != nil {
			t.Fatalf("unmarshaling result: %v", err)
		}
		if got["name"] != "hello" || got["value"] != float64(42) {
			t.Errorf("unexpected decoded body: %v", got)
		}
	})

	t.Run("schemaType AVRO explicit", func(t *testing.T) {
		srv := newSchemaRegistryStub(t, avroSchema, "AVRO")
		dec := New(srv.URL, "", "")
		raw := confluentFrame(2, body)

		_, format, err := dec.Decode(raw)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if format != "avro" {
			t.Errorf("format = %q, want avro", format)
		}
	})
}

func TestDecodeUnsupportedSchemaType(t *testing.T) {
	srv := newSchemaRegistryStub(t, `{"type":"object"}`, "JSON")
	dec := New(srv.URL, "", "")
	raw := confluentFrame(3, []byte{0x00})

	_, _, err := dec.Decode(raw)
	if err == nil {
		t.Fatal("expected error for unsupported schemaType")
	}
}

func TestDecodeNotConfluentFramed(t *testing.T) {
	dec := New("http://unused.invalid", "", "")
	_, _, err := dec.Decode([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for non-confluent-framed payload")
	}
}

