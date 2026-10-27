package sqlscan

import (
	"strings"
)

// defaultColumns maps the column names sensitive in every table to the
// label their values get. Each also matches as a suffix after "_"
// (billing_email, smtp_password, customer_first_name), except name itself,
// which would take in product_name and file_name; common person-name
// columns are listed instead.
func defaultColumns() map[string]string {
	return map[string]string{
		"email":                     "EMAIL",
		"password":                  "PASSWORD",
		"phone":                     "PHONE",
		"mobile":                    "PHONE",
		"first_name":                "NAME",
		"last_name":                 "NAME",
		"name":                      "NAME",
		"full_name":                 "NAME",
		"billing_name":              "NAME",
		"shipping_name":             "NAME",
		"customer_name":             "NAME",
		"contact_name":              "NAME",
		"display_name":              "NAME",
		"address":                   "ADDRESS",
		"postcode":                  "POSTCODE",
		"dob":                       "DOB",
		"date_of_birth":             "DOB",
		"ip_address":                "IP",
		"remember_token":            "TOKEN",
		"api_token":                 "TOKEN",
		"token":                     "TOKEN",
		"secret":                    "SECRET",
		"two_factor_secret":         "SECRET",
		"two_factor_recovery_codes": "SECRET",
	}
}

// exactOnly reports whether a default column must match exactly: "name",
// since a suffix match on _name would take in product_name.
func exactOnly(name string) bool {
	return name == "name"
}

// defaultTables maps table names to columns sensitive in that table only:
// Laravel's session IDs and payloads are live credentials.
func defaultTables() map[string][]string {
	return map[string][]string{"sessions": {"id", "payload"}}
}

// Columns decides which columns hold values to hide whole. Names compare
// without regard to case.
type Columns struct {
	defaults map[string]string
	// extra and tables hold configured names, which match exactly.
	extra  map[string]bool
	tables map[string]map[string]bool
}

// NewColumns returns the default sensitive columns plus columns, which are
// sensitive in every table, and tables, which maps a table name to columns
// sensitive in that table.
func NewColumns(columns []string, tables map[string][]string) *Columns {
	c := &Columns{defaults: defaultColumns(), extra: map[string]bool{}, tables: map[string]map[string]bool{}}
	for _, column := range columns {
		c.extra[strings.ToLower(column)] = true
	}
	for _, source := range []map[string][]string{defaultTables(), tables} {
		for table, names := range source {
			table = strings.ToLower(table)
			if c.tables[table] == nil {
				c.tables[table] = map[string]bool{}
			}
			for _, column := range names {
				c.tables[table][strings.ToLower(column)] = true
			}
		}
	}

	return c
}

// Label returns the label for values in table's column, and whether the
// column is sensitive. A default name gives its own label (billing_email
// gives EMAIL, from the longest default it ends with); a configured name
// gives the column name in upper case.
func (c *Columns) Label(table, column string) (string, bool) {
	column = strings.ToLower(column)
	if label, ok := c.defaults[column]; ok {
		return label, true
	}
	if c.extra[column] || c.tables[strings.ToLower(table)][column] {
		return columnLabel(column), true
	}

	best := ""
	for name := range c.defaults {
		if !exactOnly(name) && len(name) > len(best) && strings.HasSuffix(column, "_"+name) {
			best = name
		}
	}
	if best == "" {
		return "", false
	}

	return c.defaults[best], true
}

// columnLabel turns a column name into a placeholder label: upper case,
// with anything but letters and digits as "_".
func columnLabel(column string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, column)
}
