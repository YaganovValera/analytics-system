// query-service/internal/storage/timescaledb/interface.go
package timescaledb

import "context"

type Repository interface {
	ExecuteSQL(ctx context.Context, query string, params map[string]string) (columns []string, rows [][]string, err error)
	Ping(ctx context.Context) error
	Close()
}
