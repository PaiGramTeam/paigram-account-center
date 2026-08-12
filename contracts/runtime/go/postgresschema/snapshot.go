package postgresschema

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type Entry struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

func Capture(ctx context.Context, db *sql.DB) ([]Entry, error) {
	queries := []string{
		relationQuery,
		columnQuery,
		constraintQuery,
		indexQuery,
		triggerQuery,
		functionQuery,
		enumQuery,
		policyQuery,
		sequenceQuery,
	}
	entries := make([]Entry, 0, 256)
	for _, query := range queries {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("query PostgreSQL schema catalog: %w", err)
		}
		for rows.Next() {
			var entry Entry
			if err := rows.Scan(&entry.Kind, &entry.Name, &entry.Detail); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan PostgreSQL schema catalog: %w", err)
			}
			entries = append(entries, entry)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate PostgreSQL schema catalog: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close PostgreSQL schema catalog rows: %w", err)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Detail < entries[j].Detail
	})
	return entries, nil
}

const relationQuery = `
SELECT 'relation', quote_ident(n.nspname) || '.' || quote_ident(c.relname),
       concat('kind=', c.relkind, ';persistence=', c.relpersistence,
              ';row_security=', c.relrowsecurity, ';force_row_security=', c.relforcerowsecurity,
              CASE WHEN c.relkind IN ('v', 'm') THEN ';definition=' || pg_get_viewdef(c.oid, true) ELSE '' END)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
`

const columnQuery = `
SELECT 'column', quote_ident(n.nspname) || '.' || quote_ident(c.relname) || '.' || quote_ident(a.attname),
       concat('position=', a.attnum, ';type=', pg_catalog.format_type(a.atttypid, a.atttypmod),
              ';not_null=', a.attnotnull, ';identity=', a.attidentity, ';generated=', a.attgenerated,
              ';default=', COALESCE(pg_get_expr(d.adbin, d.adrelid, true), ''),
              ';collation=', CASE WHEN a.attcollation = t.typcollation THEN '' ELSE co.collname END)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_type t ON t.oid = a.atttypid
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
LEFT JOIN pg_collation co ON co.oid = a.attcollation
WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm')
  AND a.attnum > 0 AND NOT a.attisdropped
`

const constraintQuery = `
SELECT 'constraint', quote_ident(n.nspname) || '.' || quote_ident(c.relname) || '.' || quote_ident(con.conname),
       concat('type=', con.contype, ';deferrable=', con.condeferrable, ';deferred=', con.condeferred,
              ';validated=', con.convalidated, ';definition=', pg_get_constraintdef(con.oid, true))
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
`

const indexQuery = `
SELECT 'index', quote_ident(n.nspname) || '.' || quote_ident(t.relname) || '.' || quote_ident(i.relname),
       concat('primary=', x.indisprimary, ';unique=', x.indisunique, ';valid=', x.indisvalid,
              ';ready=', x.indisready, ';definition=', pg_get_indexdef(i.oid))
FROM pg_index x
JOIN pg_class t ON t.oid = x.indrelid
JOIN pg_class i ON i.oid = x.indexrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = 'public'
`

const triggerQuery = `
SELECT 'trigger', quote_ident(n.nspname) || '.' || quote_ident(c.relname) || '.' || quote_ident(t.tgname),
       pg_get_triggerdef(t.oid, true)
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND NOT t.tgisinternal
`

const functionQuery = `
SELECT 'function', quote_ident(n.nspname) || '.' || quote_ident(p.proname) ||
       '(' || pg_get_function_identity_arguments(p.oid) || ')', pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'public'
  AND NOT EXISTS (
      SELECT 1 FROM pg_depend d
      JOIN pg_extension e ON e.oid = d.refobjid
      WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e'
  )
`

const enumQuery = `
SELECT 'enum', quote_ident(n.nspname) || '.' || quote_ident(t.typname),
       string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder)
FROM pg_type t
JOIN pg_namespace n ON n.oid = t.typnamespace
JOIN pg_enum e ON e.enumtypid = t.oid
WHERE n.nspname = 'public'
GROUP BY n.nspname, t.typname
`

const policyQuery = `
SELECT 'policy', quote_ident(schemaname) || '.' || quote_ident(tablename) || '.' || quote_ident(policyname),
       concat('permissive=', permissive, ';roles=', array_to_string(roles, ','), ';command=', cmd,
              ';using=', COALESCE(qual, ''), ';check=', COALESCE(with_check, ''))
FROM pg_policies
WHERE schemaname = 'public'
`

const sequenceQuery = `
SELECT 'sequence', quote_ident(schemaname) || '.' || quote_ident(sequencename),
       concat('type=', data_type, ';start=', start_value, ';min=', min_value, ';max=', max_value,
              ';increment=', increment_by, ';cycle=', cycle, ';cache=', cache_size)
FROM pg_sequences
WHERE schemaname = 'public'
`
