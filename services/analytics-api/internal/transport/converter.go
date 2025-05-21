// analytics-api/internal/transport/converter.go
package transport

import (
	"context"
	"fmt"

	analyticspb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/analytics"
	"google.golang.org/protobuf/proto"
)

// MarshalCandleToBytes сериализует свечу в []byte для Kafka.
func MarshalCandleToBytes(_ context.Context, c *analyticspb.Candle) ([]byte, error) {
	return proto.Marshal(c)
}

// UnmarshalCandleFromBytes десериализует []byte в Candle.
func UnmarshalCandleFromBytes(data []byte) (*analyticspb.Candle, error) {
	var pb analyticspb.Candle
	if err := proto.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("unmarshal candle: %w", err)
	}
	return &pb, nil
}
