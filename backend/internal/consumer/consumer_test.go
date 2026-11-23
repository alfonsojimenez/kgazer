package consumer

import (
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestEncodeHeaders(t *testing.T) {
	t.Run("no headers", func(t *testing.T) {
		if got := encodeHeaders(nil); got != nil {
			t.Errorf("expected nil, got %s", got)
		}
	})

	t.Run("text, null, binary and duplicate keys preserve order", func(t *testing.T) {
		got := encodeHeaders([]kafka.Header{
			{Key: "trace", Value: []byte("abc")},
			{Key: "nil", Value: nil},
			{Key: "bin", Value: []byte{0xff, 0x00, 0x01}},
			{Key: "trace", Value: []byte("def")},
		})
		want := `[{"key":"trace","value":"abc"},{"key":"nil","value":null},{"key":"bin","value":"/wAB","base64":true},{"key":"trace","value":"def"}]`
		if string(got) != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})
}
