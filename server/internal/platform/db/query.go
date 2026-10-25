package db

import (
	"context"
	"fmt"
)

// InsertID executes an INSERT and returns the new row id.
func InsertID(ctx context.Context, q DBTX, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("inserting: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading insert id: %w", err)
	}
	return id, nil
}

// QueryIDs runs a single-column integer query and returns all values in order.
// The rows are fully read and closed before it returns.
func QueryIDs(ctx context.Context, q DBTX, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ids: %w", err)
	}
	return ids, nil
}
