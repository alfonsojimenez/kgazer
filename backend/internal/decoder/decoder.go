package decoder

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"

	"github.com/bufbuild/protocompile/linker"
	"github.com/linkedin/goavro/v2"
)

type Decoder struct {
	registryURL string
	username    string
	password    string
	mu          sync.RWMutex
	codecs      map[int32]*goavro.Codec
	schemaMetas map[int32]*schemaResponse
	protoFiles  map[int32]linker.Files
}

func New(registryURL, username, password string) *Decoder {
	return &Decoder{
		registryURL: registryURL,
		username:    username,
		password:    password,
		codecs:      make(map[int32]*goavro.Codec),
		schemaMetas: make(map[int32]*schemaResponse),
		protoFiles:  make(map[int32]linker.Files),
	}
}

// Decode transparently deserialises a Confluent Schema Registry-framed Kafka
// message (magic byte + big-endian schema ID) into JSON. It routes to the
// Avro or Protobuf decode path based on the schema's registered schemaType,
// returning the format string ("avro" or "protobuf") alongside the decoded
// JSON bytes.
func (d *Decoder) Decode(raw []byte) ([]byte, string, error) {
	if len(raw) < 5 || raw[0] != 0x00 {
		return nil, "", fmt.Errorf("not a confluent-framed message")
	}

	schemaID := int32(binary.BigEndian.Uint32(raw[1:5]))
	payload := raw[5:]

	meta, err := d.getSchemaMeta(schemaID)
	if err != nil {
		return nil, "", fmt.Errorf("fetching schema %d: %w", schemaID, err)
	}

	switch meta.SchemaType {
	case "", "AVRO":
		codec, err := d.getCodec(schemaID)
		if err != nil {
			return nil, "", fmt.Errorf("fetching schema %d: %w", schemaID, err)
		}

		native, _, err := codec.NativeFromBinary(payload)
		if err != nil {
			return nil, "", fmt.Errorf("decoding avro: %w", err)
		}

		jsonBytes, err := json.Marshal(sanitize(native))
		if err != nil {
			return nil, "", fmt.Errorf("marshaling avro json: %w", err)
		}
		return jsonBytes, "avro", nil

	case "PROTOBUF":
		jsonBytes, err := d.decodeProtobuf(schemaID, meta, payload)
		if err != nil {
			return nil, "", fmt.Errorf("decoding protobuf: %w", err)
		}
		return jsonBytes, "protobuf", nil

	default:
		return nil, "", fmt.Errorf("unsupported schemaType %q", meta.SchemaType)
	}
}

func sanitize(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 1 {
			for k, child := range val {
				if isAvroUnionKey(k) {
					return sanitize(child)
				}
			}
		}
		for k, child := range val {
			val[k] = sanitize(child)
		}
		return val
	case []interface{}:
		for i, child := range val {
			val[i] = sanitize(child)
		}
		return val
	case *big.Rat:
		f, _ := val.Float64()
		return f
	default:
		return v
	}
}

var avroTypeNames = map[string]bool{
	"null": true, "boolean": true, "int": true, "long": true,
	"float": true, "double": true, "bytes": true, "string": true,
	"array": true, "map": true, "enum": true, "fixed": true,
}

func isAvroUnionKey(key string) bool {
	if avroTypeNames[key] {
		return true
	}
	for _, c := range key {
		if c == '.' {
			return true
		}
	}
	return false
}

func (d *Decoder) getCodec(id int32) (*goavro.Codec, error) {
	d.mu.RLock()
	if codec, ok := d.codecs[id]; ok {
		d.mu.RUnlock()
		return codec, nil
	}
	d.mu.RUnlock()

	meta, err := d.getSchemaMeta(id)
	if err != nil {
		return nil, err
	}

	codec, err := goavro.NewCodec(meta.Schema)
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}

	d.mu.Lock()
	d.codecs[id] = codec
	d.mu.Unlock()

	return codec, nil
}

type schemaResponse struct {
	Schema     string           `json:"schema"`
	SchemaType string           `json:"schemaType"`
	References []schemaRefEntry `json:"references"`
}

type schemaRefEntry struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Version int    `json:"version"`
}

// getSchemaMeta fetches (and caches) the full schema registry response for a
// schema ID, including its schemaType, which is used to route decoding to
// the Avro or Protobuf path.
func (d *Decoder) getSchemaMeta(id int32) (*schemaResponse, error) {
	d.mu.RLock()
	if meta, ok := d.schemaMetas[id]; ok {
		d.mu.RUnlock()
		return meta, nil
	}
	d.mu.RUnlock()

	meta, err := d.fetchSchemaMeta(id)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.schemaMetas[id] = meta
	d.mu.Unlock()

	return meta, nil
}

func (d *Decoder) fetchSchemaMeta(id int32) (*schemaResponse, error) {
	url := fmt.Sprintf("%s/schemas/ids/%d", d.registryURL, id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.schemaregistry.v1+json")
	if d.username != "" {
		req.SetBasicAuth(d.username, d.password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("schema registry returned %d: %s", resp.StatusCode, body)
	}

	var sr schemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &sr, nil
}

// fetchSchemaBySubjectVersion fetches a schema by subject and version, used
// when resolving cross-subject protobuf references (the "references" field
// of a schema registry response).
func (d *Decoder) fetchSchemaBySubjectVersion(subject string, version int) (*schemaResponse, error) {
	url := fmt.Sprintf("%s/subjects/%s/versions/%d", d.registryURL, subject, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.schemaregistry.v1+json")
	if d.username != "" {
		req.SetBasicAuth(d.username, d.password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting referenced schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("schema registry returned %d: %s", resp.StatusCode, body)
	}

	var sr schemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &sr, nil
}
