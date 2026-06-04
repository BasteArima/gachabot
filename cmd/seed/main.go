// Command seed applies a declarative theme file to the database, or exports the
// current database content back into a theme file.
//
//	seed import <theme.json> [--dry-run] [--reset]
//	seed export <theme.json>
//	seed validate <theme.json>
//
// It reads the same POSTGRES_* environment (and .env) as the bot.
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"gachabot/internal/config"
	"gachabot/internal/theme"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	log.SetFlags(0)
	_ = godotenv.Load()

	args := os.Args[1:]
	if len(args) < 2 {
		usage()
	}
	cmd, file := args[0], args[1]
	flags := args[2:]

	switch cmd {
	case "validate":
		cmdValidate(file)
	case "import":
		cmdImport(file, hasFlag(flags, "--dry-run"), hasFlag(flags, "--reset"))
	case "export":
		cmdExport(file)
	default:
		usage()
	}
}

func usage() {
	log.Fatal(`usage:
  seed validate <theme.json>
  seed import   <theme.json> [--dry-run] [--reset]
  seed export   <theme.json>`)
}

func hasFlag(flags []string, name string) bool {
	for _, f := range flags {
		if f == name {
			return true
		}
	}
	return false
}

func cmdValidate(file string) {
	t, err := theme.Load(file)
	if err != nil {
		log.Fatal(err)
	}
	warns, err := t.Validate()
	printWarnings(warns)
	if err != nil {
		log.Fatalf("INVALID: %v", err)
	}
	log.Printf("OK: %d rarities, %d sets, %d cards", len(t.Rarities), len(t.Sets), len(t.Cards))
}

func cmdImport(file string, dryRun, reset bool) {
	t, err := theme.Load(file)
	if err != nil {
		log.Fatal(err)
	}
	warns, err := t.Validate()
	printWarnings(warns)
	if err != nil {
		log.Fatalf("INVALID: %v", err)
	}

	if reset && !dryRun {
		confirmReset()
	}

	db := connect()
	defer db.Close()

	rep, err := theme.Import(db, t, theme.Options{DryRun: dryRun, Reset: reset})
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}
	log.Println(rep.String())
}

func cmdExport(file string) {
	db := connect()
	defer db.Close()

	t, err := theme.Export(db)
	if err != nil {
		log.Fatalf("export failed: %v", err)
	}

	// "-" writes the JSON to stdout (status goes to stderr via log), so it can be
	// copied straight out of a container console into the web editor.
	if file == "-" {
		data, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(data))
		log.Printf("exported %d rarities, %d sets, %d cards", len(t.Rarities), len(t.Sets), len(t.Cards))
		return
	}

	if err := t.Save(file); err != nil {
		log.Fatal(err)
	}
	log.Printf("exported %d rarities, %d sets, %d cards -> %s", len(t.Rarities), len(t.Sets), len(t.Cards), file)
}

func confirmReset() {
	fmt.Print(`
!!! --reset DELETES all rarities, sets and cards, and CASCADES to player
    collections (inventories, fragments, pity, unlocked sets).
    Use only on an empty or dev database.

Type RESET to continue: `)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != "RESET" {
		log.Fatal("aborted")
	}
}

func printWarnings(warns []string) {
	for _, w := range warns {
		log.Printf("WARN: %s", w)
	}
}

func connect() *sql.DB {
	db, err := sql.Open("postgres", config.PostgresFromEnv().DSN())
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	return db
}
