package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type input struct {
	Table  string         `json:"table"`
	Select []string       `json:"select"`
	Where  map[string]any `json:"where"`
	Limit  int            `json:"limit"`
}

type output struct {
	Table   string           `json:"table"`
	Count   int              `json:"count"`
	Limited bool             `json:"limited"`
	Rows    []map[string]any `json:"rows"`
}

type table struct {
	Columns []string
	Rows    []map[string]any
}

var tables = map[string]table{
	"customers": {
		Columns: []string{"id", "name", "tier", "region", "active"},
		Rows: []map[string]any{
			{"id": "cus_001", "name": "Acme Robotics", "tier": "enterprise", "region": "eu", "active": true},
			{"id": "cus_002", "name": "Northstar Labs", "tier": "startup", "region": "us", "active": true},
			{"id": "cus_003", "name": "Kaya Logistics", "tier": "business", "region": "tr", "active": false},
		},
	},
	"tickets": {
		Columns: []string{"id", "customer_id", "severity", "status", "owner"},
		Rows: []map[string]any{
			{"id": "tic_101", "customer_id": "cus_001", "severity": "high", "status": "open", "owner": "infra"},
			{"id": "tic_102", "customer_id": "cus_002", "severity": "low", "status": "closed", "owner": "support"},
			{"id": "tic_103", "customer_id": "cus_001", "severity": "high", "status": "open", "owner": "security"},
			{"id": "tic_104", "customer_id": "cus_003", "severity": "medium", "status": "open", "owner": "support"},
		},
	},
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		fail(err)
	}
	out, err := query(in)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fail(err)
	}
}

func query(in input) (output, error) {
	t, ok := tables[in.Table]
	if !ok {
		return output{}, fmt.Errorf("unknown table %q", in.Table)
	}
	columns, err := selectedColumns(in.Select, t.Columns)
	if err != nil {
		return output{}, err
	}
	if err := validateWhere(in.Where, t.Columns); err != nil {
		return output{}, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = 10
	}

	rows := []map[string]any{}
	matched := 0
	for _, row := range t.Rows {
		if !matches(row, in.Where) {
			continue
		}
		matched++
		if len(rows) >= limit {
			continue
		}
		projected := map[string]any{}
		for _, column := range columns {
			projected[column] = row[column]
		}
		rows = append(rows, projected)
	}

	return output{Table: in.Table, Count: len(rows), Limited: matched > len(rows), Rows: rows}, nil
}

func selectedColumns(selectColumns, available []string) ([]string, error) {
	if len(selectColumns) == 1 && selectColumns[0] == "*" {
		return append([]string(nil), available...), nil
	}
	for _, column := range selectColumns {
		if !contains(available, column) {
			return nil, fmt.Errorf("unknown selected column %q", column)
		}
	}
	return selectColumns, nil
}

func validateWhere(where map[string]any, available []string) error {
	for column := range where {
		if !contains(available, column) {
			return fmt.Errorf("unknown filter column %q", column)
		}
	}
	return nil
}

func matches(row map[string]any, where map[string]any) bool {
	for column, expected := range where {
		if !valuesEqual(row[column], expected) {
			return false
		}
	}
	return true
}

func valuesEqual(actual, expected any) bool {
	switch want := expected.(type) {
	case nil:
		return actual == nil
	case string:
		got, ok := actual.(string)
		return ok && got == want
	case bool:
		got, ok := actual.(bool)
		return ok && got == want
	case float64:
		got, ok := numericValue(actual)
		return ok && got == want
	default:
		return false
	}
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
