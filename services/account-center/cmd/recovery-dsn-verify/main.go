package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: recovery-dsn-verify <dsn-file> <database>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read DSN file")
		os.Exit(1)
	}
	if err := validateDSN(string(raw), os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config, _ := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	connection := config.ConnConfig
	fmt.Printf("%s|%d|%s\n", connection.Host, connection.Port, connection.Database)
}

func validateDSN(raw string, expectedDatabase string) error {
	config, err := pgxpool.ParseConfig(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("could not parse PostgreSQL DSN")
	}
	connection := config.ConnConfig
	if connection.Host != "postgres" || connection.Port != 5432 || connection.Database != expectedDatabase {
		return errors.New("PostgreSQL DSN targets an unexpected service")
	}
	for _, fallback := range connection.Fallbacks {
		if fallback.Host != "postgres" || fallback.Port != 5432 {
			return errors.New("PostgreSQL DSN has an unexpected fallback service")
		}
	}
	return nil
}
