package decoder

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"

	"github.com/linkedin/goavro/v2"
)

type Decoder struct {
	registryURL string
	username    string
	password    string
	mu          sync.RWMutex
	codecs      map[int32]*goavro.Codec
}

func New(registryURL, username, password string) *Decoder {
	return &Decoder{
		registryURL: registryURL,
		username:    username,
		password:    password,
		codecs:      make(map[int32]*goavro.Codec),
	}
}

func (d *Decoder) Decode(raw []byte) ([]byte, error) {
	if len(raw) < 5 || raw[0] != 0x00 {
		return nil, fmt.Errorf("not an avro message")
	}

	schemaID := int32(binary.BigEndian.Uint32(raw[1:5]))
	payload := raw[5:]

	codec, err := d.getCodec(schemaID)
	if err != nil {
		return nil, fmt.Errorf("fetching schema %d: %w", schemaID, err)
	}

	native, _, err := codec.NativeFromBinary(payload)
	if err != nil {
		return nil, fmt.Errorf("decoding avro: %w", err)
	}

	return json.Marshal(sanitize(native))
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

var avroPrimitiveTypes = map[string]bool{
	"null": true, "boolean": true, "int": true, "long": true,
	"float": true, "double": true, "bytes": true, "string": true,
}

func isAvroUnionKey(key string) bool {
	if avroPrimitiveTypes[key] {
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

	schema, err := d.fetchSchema(id)
	if err != nil {
		return nil, err
	}

	codec, err := goavro.NewCodec(schema)
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}

	d.mu.Lock()
	d.codecs[id] = codec
	d.mu.Unlock()

	return codec, nil
}

type schemaResponse struct {
	Schema string `json:"schema"`
}

func (d *Decoder) fetchSchema(id int32) (string, error) {
	url := fmt.Sprintf("%s/schemas/ids/%d", d.registryURL, id)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.schemaregistry.v1+json")
	if d.username != "" {
		req.SetBasicAuth(d.username, d.password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("schema registry returned %d: %s", resp.StatusCode, body)
	}

	var sr schemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return sr.Schema, nil
}
