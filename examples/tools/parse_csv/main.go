package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type input struct {
	CSV string `json:"csv"`
}

type output struct {
	Rows    int      `json:"rows"`
	Columns []string `json:"columns"`
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
	records, err := csv.NewReader(strings.NewReader(in.CSV)).ReadAll()
	if err != nil {
		fail(err)
	}
	out := output{}
	if len(records) > 0 {
		out.Columns = records[0]
		out.Rows = len(records) - 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
