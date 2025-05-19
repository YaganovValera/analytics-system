// common/interval/interval.go
package interval

import (
	"fmt"
	"time"

	commonpb "github.com/YaganovValera/analytics-system/proto/gen/go/v1/common"
)

type Interval string

const (
	Interval1m  Interval = "1m"
	Interval5m  Interval = "5m"
	Interval15m Interval = "15m"
	Interval1h  Interval = "1h"
	Interval4h  Interval = "4h"
	Interval1d  Interval = "1d"
)

var protoToInternal = map[commonpb.AggregationInterval]Interval{
	commonpb.AggregationInterval_AGG_INTERVAL_1_MINUTE:   Interval1m,
	commonpb.AggregationInterval_AGG_INTERVAL_5_MINUTES:  Interval5m,
	commonpb.AggregationInterval_AGG_INTERVAL_15_MINUTES: Interval15m,
	commonpb.AggregationInterval_AGG_INTERVAL_1_HOUR:     Interval1h,
	commonpb.AggregationInterval_AGG_INTERVAL_4_HOURS:    Interval4h,
	commonpb.AggregationInterval_AGG_INTERVAL_1_DAY:      Interval1d,
}

var internalToProto = map[Interval]commonpb.AggregationInterval{
	Interval1m:  commonpb.AggregationInterval_AGG_INTERVAL_1_MINUTE,
	Interval5m:  commonpb.AggregationInterval_AGG_INTERVAL_5_MINUTES,
	Interval15m: commonpb.AggregationInterval_AGG_INTERVAL_15_MINUTES,
	Interval1h:  commonpb.AggregationInterval_AGG_INTERVAL_1_HOUR,
	Interval4h:  commonpb.AggregationInterval_AGG_INTERVAL_4_HOURS,
	Interval1d:  commonpb.AggregationInterval_AGG_INTERVAL_1_DAY,
}

var intervalDurations = map[Interval]time.Duration{
	Interval1m:  time.Minute,
	Interval5m:  5 * time.Minute,
	Interval15m: 15 * time.Minute,
	Interval1h:  time.Hour,
	Interval4h:  4 * time.Hour,
	Interval1d:  24 * time.Hour,
}

// FromProto конвертирует protobuf-значение в внутренний Interval.
func FromProto(i commonpb.AggregationInterval) (Interval, error) {
	if val, ok := protoToInternal[i]; ok {
		return val, nil
	}
	return "", fmt.Errorf("unknown AggregationInterval: %v", i)
}

// ToProto конвертирует внутренний Interval в protobuf-значение.
func ToProto(i Interval) (commonpb.AggregationInterval, error) {
	if val, ok := internalToProto[i]; ok {
		return val, nil
	}
	return commonpb.AggregationInterval_AGG_INTERVAL_UNSPECIFIED, fmt.Errorf("unknown Interval: %v", i)
}

// Duration возвращает длительность интервала.
func Duration(i Interval) (time.Duration, error) {
	if d, ok := intervalDurations[i]; ok {
		return d, nil
	}
	return 0, fmt.Errorf("unknown Interval: %v", i)
}
