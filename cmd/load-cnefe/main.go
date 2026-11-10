// load-cnefe loads a state's CNEFE (IBGE, Censo 2022) into the geocoder's address list
// (docs/geocoding-plan.md): numbers on streets, and the places addresses with no number give by
// quadra, lote or casa. It reads the file IBGE publishes for a state, zipped (such as 31_MG.zip) or
// not. Loading a state again replaces it. Dry run unless given -commit. It takes a few gigabytes of
// memory while reducing a large state's file. Running servers find a newly loaded list within a
// minute.
//
//	load-cnefe -file 31_MG.zip            # counts only
//	load-cnefe -file 31_MG.zip -commit
package main

import (
	"archive/zip"
	"context"
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()
	file := flag.String("file", "", "the file IBGE publishes for a state, .zip or .csv")
	commit := cmdutil.CommitFlag()
	flag.Parse()
	if *file == "" {
		log.Fatal("-file is required")
	}

	content, closeFile := open(*file)
	defer closeFile()
	data, stats, err := geocode.ReadCNEFE(content)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("rows=%d kept=%d located=%d no_number=%d coarse=%d bad=%d numbers=%d places=%d",
		stats.Rows, stats.Kept, stats.Located, stats.NoNumber, stats.Coarse, stats.BadRow, stats.Entries, stats.Places)
	if !*commit {
		log.Print("dry run: pass -commit to load")
		return
	}
	db := cmdutil.OpenDB()
	defer db.Close()
	if err := geocode.LoadCNEFE(context.Background(), db, data); err != nil {
		log.Fatal(err)
	}
	log.Print("loaded")
}

// open reads a .zip holding one file, or the file itself.
func open(path string) (io.Reader, func()) {
	if !strings.HasSuffix(strings.ToLower(path), ".zip") {
		f, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		return f, func() { f.Close() }
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		log.Fatal(err)
	}
	if len(archive.File) != 1 {
		log.Fatalf("%s: expected one file inside, found %d", path, len(archive.File))
	}
	content, err := archive.File[0].Open()
	if err != nil {
		log.Fatal(err)
	}
	return content, func() { content.Close(); archive.Close() }
}
