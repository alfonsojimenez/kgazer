package decoder

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// mainProtoFile is the synthetic filename used when compiling the schema
// registry's in-memory .proto source for a given schema ID.
const mainProtoFile = "__main.proto"

// decodeProtobuf decodes a Confluent wire-format Protobuf payload (the bytes
// after the magic byte + schema ID) into JSON. It compiles the schema's
// .proto source (caching the result per schema ID), reads the message-index
// varint array to find the target message descriptor, then unmarshals the
// payload into a dynamicpb message and marshals it to JSON.
func (d *Decoder) decodeProtobuf(schemaID int32, meta *schemaResponse, payload []byte) ([]byte, error) {
	files, err := d.getProtoFiles(schemaID, meta)
	if err != nil {
		return nil, fmt.Errorf("compiling proto schema: %w", err)
	}

	bytesRead, indexes, err := readMessageIndexes(payload)
	if err != nil {
		return nil, fmt.Errorf("reading message indexes: %w", err)
	}
	body := payload[bytesRead:]

	mainFile := files.FindFileByPath(mainProtoFile)
	if mainFile == nil {
		return nil, fmt.Errorf("compiled schema is missing %s", mainProtoFile)
	}

	md, err := resolveMessageDescriptor(mainFile.Messages(), indexes)
	if err != nil {
		return nil, fmt.Errorf("resolving message descriptor: %w", err)
	}

	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil, fmt.Errorf("unmarshaling protobuf payload: %w", err)
	}

	// UseProtoNames is left false (the default) to emit lowerCamelCase JSON
	// field names, matching idiomatic protojson output. This differs from the
	// Avro path, which preserves raw Avro field names as defined by the
	// schema - a deliberate, documented divergence rather than an oversight.
	jsonBytes, err := protojson.MarshalOptions{Resolver: files.AsResolver()}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshaling protobuf json: %w", err)
	}

	return jsonBytes, nil
}

// getProtoFiles returns the compiled linker.Files for a schema ID, compiling
// and caching them on first use.
func (d *Decoder) getProtoFiles(schemaID int32, meta *schemaResponse) (linker.Files, error) {
	d.mu.RLock()
	if files, ok := d.protoFiles[schemaID]; ok {
		d.mu.RUnlock()
		return files, nil
	}
	d.mu.RUnlock()

	sources := map[string]string{mainProtoFile: meta.Schema}
	if err := d.collectReferences(meta.References, sources); err != nil {
		return nil, fmt.Errorf("resolving schema references: %w", err)
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(sources),
		}),
	}

	files, err := compiler.Compile(context.Background(), mainProtoFile)
	if err != nil {
		return nil, fmt.Errorf("parsing proto source: %w", err)
	}

	d.mu.Lock()
	d.protoFiles[schemaID] = files
	d.mu.Unlock()

	return files, nil
}

// collectReferences recursively fetches cross-subject schema references and
// adds them to the sources map, keyed by the literal import name used in the
// referencing schema. It is a no-op (and makes no network calls) when refs
// is empty, keeping the common single-file schema case allocation-light.
func (d *Decoder) collectReferences(refs []schemaRefEntry, sources map[string]string) error {
	for _, ref := range refs {
		if _, ok := sources[ref.Name]; ok {
			continue
		}
		refMeta, err := d.fetchSchemaBySubjectVersion(ref.Subject, ref.Version)
		if err != nil {
			return fmt.Errorf("fetching reference %s: %w", ref.Name, err)
		}
		sources[ref.Name] = refMeta.Schema
		if err := d.collectReferences(refMeta.References, sources); err != nil {
			return err
		}
	}
	return nil
}

// readMessageIndexes reads the Confluent message-index array from the start
// of a protobuf payload. It uses zigzag-varint encoding (encoding/binary's
// Varint scheme), the same as confluent-kafka-go's serde implementation. The
// first varint is the array length; a length of 0 is shorthand for [0] (the
// first top-level message in the file, the common case). It returns the
// number of bytes consumed and the decoded index path.
func readMessageIndexes(payload []byte) (int, []int, error) {
	arrayLen, bytesRead := binary.Varint(payload)
	if bytesRead <= 0 {
		return bytesRead, nil, fmt.Errorf("unable to read message indexes")
	}
	if arrayLen == 0 {
		return bytesRead, []int{0}, nil
	}
	msgIndexes := make([]int, arrayLen)
	for i := 0; i < int(arrayLen); i++ {
		idx, read := binary.Varint(payload[bytesRead:])
		if read <= 0 {
			return bytesRead, nil, fmt.Errorf("unable to read message indexes")
		}
		bytesRead += read
		msgIndexes[i] = int(idx)
	}
	return bytesRead, msgIndexes, nil
}

// resolveMessageDescriptor walks a message-index path (top-level index, then
// nested-message index, recursing as needed) to find the target descriptor.
func resolveMessageDescriptor(messages protoreflect.MessageDescriptors, indexes []int) (protoreflect.MessageDescriptor, error) {
	if len(indexes) == 0 {
		return nil, fmt.Errorf("empty message index path")
	}
	idx := indexes[0]
	if idx < 0 || idx >= messages.Len() {
		return nil, fmt.Errorf("message index %d out of range (have %d)", idx, messages.Len())
	}
	md := messages.Get(idx)
	if len(indexes) == 1 {
		return md, nil
	}
	return resolveMessageDescriptor(md.Messages(), indexes[1:])
}
