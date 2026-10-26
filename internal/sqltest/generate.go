package sqltest

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// WriteDump writes a Laravel-shaped dump of users, orders and sessions in
// dialect d, with users users, two orders each and one session each. Output
// is deterministic and shaped like the dialect's dump tool (mysqldump
// extended inserts, pg_dump COPY blocks, sqlite3 .dump inserts), so tests
// and benchmarks can stream input of any size without committing it.
func WriteDump(w io.Writer, d Dialect, users int) error {
	g := &dumpWriter{w: bufio.NewWriter(w), dialect: d}

	switch d {
	case MySQL:
		g.mysql(users)
	case Postgres:
		g.postgres(users)
	case SQLite:
		g.sqlite(users)
	default:
		return fmt.Errorf("unknown dialect %q", d)
	}
	if g.err != nil {
		return g.err
	}

	return g.w.Flush()
}

// mysqlBatch is how many rows share one INSERT, like mysqldump's extended
// inserts.
const mysqlBatch = 100

type dumpWriter struct {
	w       *bufio.Writer
	dialect Dialect
	err     error
}

// printf writes unless an earlier write failed, so callers check once.
func (g *dumpWriter) printf(format string, args ...any) {
	if g.err == nil {
		_, g.err = fmt.Fprintf(g.w, format, args...)
	}
}

// generatedRow is one row's values, already formatted for the dialect.
type generatedRow []string

func (g *dumpWriter) userRow(n int) generatedRow {
	token := g.str(Fake(60, n))
	if n%5 == 0 {
		token = g.null()
	}

	return generatedRow{
		fmt.Sprint(n),
		g.str(fmt.Sprintf("User %d O'Neill", n)),
		g.str(fmt.Sprintf("user%d@shop.test", n)),
		g.str("$2y$12$" + Fake(53, n)),
		token,
		g.str("2026-10-01 09:00:00"),
	}
}

func (g *dumpWriter) orderRow(n, user int) generatedRow {
	return generatedRow{
		fmt.Sprint(n),
		fmt.Sprint(user),
		g.str(fmt.Sprintf("user%d@shop.test", user)),
		g.str(fmt.Sprintf("07700 900%03d", n%1000)),
		g.str(fmt.Sprintf("Call before delivery.\nPaid with card 4242 4242 4242 4242, ref pi_%s", Fake(24, n))),
	}
}

func (g *dumpWriter) sessionRow(user int) generatedRow {
	return generatedRow{
		g.str(Fake(32, user) + fmt.Sprintf("%08d", user)),
		fmt.Sprint(user),
		g.str(fmt.Sprintf("203.0.113.%d", user%250+1)),
		g.str("YTozOntzOjY6Il90b2tlbiI7czo0MDoi" + Fake(80, user)),
		fmt.Sprint(1793000000 + user),
	}
}

// str formats s as a string literal for INSERT statements, or as a COPY
// field for PostgreSQL.
func (g *dumpWriter) str(s string) string {
	switch g.dialect {
	case MySQL:
		return "'" + mysqlEscaper.Replace(s) + "'"
	case Postgres:
		return copyEscaper.Replace(s)
	}

	return sqliteString(s)
}

func (g *dumpWriter) null() string {
	if g.dialect == Postgres {
		return `\N`
	}

	return "NULL"
}

var (
	// mysqlEscaper escapes a string the way mysqldump does.
	mysqlEscaper = strings.NewReplacer("\\", `\\`, "'", `\'`, "\"", `\"`, "\n", `\n`, "\r", `\r`, "\x00", `\0`, "\x1a", `\Z`)
	// copyEscaper escapes a field for pg_dump's COPY text format.
	copyEscaper = strings.NewReplacer("\\", `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)
)

// sqliteString quotes s as sqlite3 .dump does: quotes doubled, and newlines
// written as \n inside replace(..., '\n', char(10)).
func sqliteString(s string) string {
	quoted := "'" + strings.ReplaceAll(s, "'", "''") + "'"
	if !strings.Contains(s, "\n") {
		return quoted
	}

	return "replace(" + strings.ReplaceAll(quoted, "\n", `\n`) + `,'\n',char(10))`
}

func (g *dumpWriter) mysql(users int) {
	g.printf("-- MySQL dump 10.13  Distrib 8.4.3, for Linux (x86_64)\n\n/*!40101 SET NAMES utf8mb4 */;\n/*!40103 SET TIME_ZONE='+00:00' */;\n\n")
	g.printf("CREATE TABLE `users` (\n  `id` bigint unsigned NOT NULL AUTO_INCREMENT,\n  `name` varchar(255) NOT NULL,\n  `email` varchar(255) NOT NULL,\n  `password` varchar(255) NOT NULL,\n  `remember_token` varchar(100) DEFAULT NULL,\n  `created_at` timestamp NULL DEFAULT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;\n\n")
	g.printf("CREATE TABLE `orders` (\n  `id` bigint unsigned NOT NULL AUTO_INCREMENT,\n  `user_id` bigint unsigned NOT NULL,\n  `billing_email` varchar(255) NOT NULL,\n  `billing_phone` varchar(32) DEFAULT NULL,\n  `notes` text,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;\n\n")
	g.printf("CREATE TABLE `sessions` (\n  `id` varchar(255) NOT NULL,\n  `user_id` bigint unsigned DEFAULT NULL,\n  `ip_address` varchar(45) DEFAULT NULL,\n  `payload` longtext NOT NULL,\n  `last_activity` int NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;\n\n")

	g.mysqlTable("users", users, func(i int) []generatedRow { return []generatedRow{g.userRow(i)} })
	g.mysqlTable("orders", users, func(i int) []generatedRow { return []generatedRow{g.orderRow(2*i-1, i), g.orderRow(2*i, i)} })
	g.mysqlTable("sessions", users, func(i int) []generatedRow { return []generatedRow{g.sessionRow(i)} })
	g.printf("-- Dump completed\n")
}

// mysqlTable writes the rows for users 1 to n in extended inserts of up to
// mysqlBatch rows each.
func (g *dumpWriter) mysqlTable(table string, n int, rows func(user int) []generatedRow) {
	g.printf("LOCK TABLES `%s` WRITE;\n", table)
	written := 0
	for user := 1; user <= n; user++ {
		for _, row := range rows(user) {
			if written%mysqlBatch == 0 {
				if written > 0 {
					g.printf(";\n")
				}
				g.printf("INSERT INTO `%s` VALUES ", table)
			} else {
				g.printf(",")
			}
			g.printf("(%s)", strings.Join(row, ","))
			written++
		}
	}
	if written > 0 {
		g.printf(";\n")
	}
	g.printf("UNLOCK TABLES;\n\n")
}

func (g *dumpWriter) postgres(users int) {
	g.printf("--\n-- PostgreSQL database dump\n--\n\n\\restrict %s\n\nSET client_encoding = 'UTF8';\nSET standard_conforming_strings = on;\n\n", Fake(64, 0))
	g.printf("CREATE TABLE public.users (\n    id bigint NOT NULL,\n    name character varying(255) NOT NULL,\n    email character varying(255) NOT NULL,\n    password character varying(255) NOT NULL,\n    remember_token character varying(100),\n    created_at timestamp(0) without time zone\n);\n\n")
	g.printf("CREATE TABLE public.orders (\n    id bigint NOT NULL,\n    user_id bigint NOT NULL,\n    billing_email character varying(255) NOT NULL,\n    billing_phone character varying(32),\n    notes text\n);\n\n")
	g.printf("CREATE TABLE public.sessions (\n    id character varying(255) NOT NULL,\n    user_id bigint,\n    ip_address character varying(45),\n    payload text NOT NULL,\n    last_activity integer NOT NULL\n);\n\n")

	g.copyTable("users (id, name, email, password, remember_token, created_at)", users, func(i int) []generatedRow { return []generatedRow{g.userRow(i)} })
	g.copyTable("orders (id, user_id, billing_email, billing_phone, notes)", users, func(i int) []generatedRow { return []generatedRow{g.orderRow(2*i-1, i), g.orderRow(2*i, i)} })
	g.copyTable("sessions (id, user_id, ip_address, payload, last_activity)", users, func(i int) []generatedRow { return []generatedRow{g.sessionRow(i)} })
	g.printf("--\n-- PostgreSQL database dump complete\n--\n\n\\unrestrict %s\n", Fake(64, 0))
}

func (g *dumpWriter) copyTable(header string, n int, rows func(user int) []generatedRow) {
	g.printf("COPY public.%s FROM stdin;\n", header)
	for user := 1; user <= n; user++ {
		for _, row := range rows(user) {
			g.printf("%s\n", strings.Join(row, "\t"))
		}
	}
	g.printf("\\.\n\n")
}

func (g *dumpWriter) sqlite(users int) {
	g.printf("PRAGMA foreign_keys=OFF;\nBEGIN TRANSACTION;\n")
	g.printf(`CREATE TABLE IF NOT EXISTS "users" ("id" integer primary key autoincrement not null, "name" varchar not null, "email" varchar not null, "password" varchar not null, "remember_token" varchar, "created_at" datetime);` + "\n")
	g.printf(`CREATE TABLE IF NOT EXISTS "orders" ("id" integer primary key autoincrement not null, "user_id" integer not null, "billing_email" varchar not null, "billing_phone" varchar, "notes" text);` + "\n")
	g.printf(`CREATE TABLE IF NOT EXISTS "sessions" ("id" varchar not null, "user_id" integer, "ip_address" varchar, "payload" text not null, "last_activity" integer not null, primary key ("id"));` + "\n")
	for user := 1; user <= users; user++ {
		g.printf("INSERT INTO users VALUES(%s);\n", strings.Join(g.userRow(user), ","))
	}
	for user := 1; user <= users; user++ {
		g.printf("INSERT INTO orders VALUES(%s);\n", strings.Join(g.orderRow(2*user-1, user), ","))
		g.printf("INSERT INTO orders VALUES(%s);\n", strings.Join(g.orderRow(2*user, user), ","))
	}
	for user := 1; user <= users; user++ {
		g.printf("INSERT INTO sessions VALUES(%s);\n", strings.Join(g.sessionRow(user), ","))
	}
	g.printf("COMMIT;\n")
}
