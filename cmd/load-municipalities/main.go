// load-municipalities loads IBGE municipality boundaries into municipalities, from the GeoJSON
// tools/municipalities/fetch.py writes (geobr; properties code, name, uf; WGS 84). Locations
// then take their municipality_code from their point. Defaults to a dry run; pass -commit to
// write.
//
//	go run ./cmd/load-municipalities -file tools/municipalities/municipalities.geojson -commit
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

var codePattern = regexp.MustCompile(`^[0-9]{7}$`)

type feature struct {
	Properties struct {
		Code string `json:"code"`
		Name string `json:"name"`
		UF   string `json:"uf"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

func main() {
	env.Load()
	file := flag.String("file", "tools/municipalities/municipalities.geojson", "GeoJSON from tools/municipalities/fetch.py")
	source := flag.String("source", "geobr read_municipality, year 2022, simplified", "where the boundaries came from (stored per row)")
	commit := cmdutil.CommitFlag()
	flag.Parse()

	data, err := os.ReadFile(*file)
	cmdutil.Fatal(err)
	var fc struct {
		Features []feature `json:"features"`
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		log.Fatalf("%s: %v", *file, err)
	}
	for i, f := range fc.Features {
		if !codePattern.MatchString(f.Properties.Code) || f.Properties.Name == "" || len(f.Properties.UF) != 2 || len(f.Geometry) == 0 {
			log.Fatalf("%s: feature %d: bad properties %+v", *file, i, f.Properties)
		}
	}
	if !*commit {
		fmt.Printf("dry run: %d municipalities in %s; pass -commit to load them\n", len(fc.Features), *file)
		return
	}

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	ctx := context.Background()
	tx, err := sqlDB.BeginTx(ctx, nil)
	cmdutil.Fatal(err)
	defer tx.Rollback()
	q := db.New(tx)
	for _, f := range fc.Features {
		if err := q.UpsertMunicipality(ctx, db.UpsertMunicipalityParams{
			Code: f.Properties.Code, Name: f.Properties.Name, Uf: f.Properties.UF,
			Geojson: string(f.Geometry), Source: *source,
		}); err != nil {
			log.Fatalf("municipality %s (%s): %v", f.Properties.Code, f.Properties.Name, err)
		}
	}
	cmdutil.Fatal(tx.Commit())
	n, err := db.New(sqlDB).CountMunicipalities(ctx)
	cmdutil.Fatal(err)
	fmt.Printf("loaded %d municipalities; %d in the table\n", len(fc.Features), n)
}
