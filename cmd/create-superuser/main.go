// create-superuser creates a SUPER_ADMIN account, the way a deployment gets its first one:
// there is no public sign-up, and every other account is created by an admin in the app
// (POST /api/admin/users). The password is read from TRACKID_SUPERUSER_PASSWORD or, when that
// is unset, from the first line of standard input, so it never appears in the shell history:
//
//	read -rs PW && echo "$PW" | create-superuser -nome Ana -ultimo-nome Souza -matricula 0001 -cargo Perita -username ana.souza
//
// With -matricula naming an account that already exists, it promotes that account to
// SUPER_ADMIN (and reactivates it) instead, leaving its password as it is.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
)

func main() {
	env.Load()

	nome := flag.String("nome", "", "first name")
	ultimoNome := flag.String("ultimo-nome", "", "last name")
	matricula := flag.String("matricula", "", "registration number, used to sign in (required)")
	cargo := flag.String("cargo", "", "job title")
	username := flag.String("username", "", "username, which names the user on every decision they sign and never changes")
	examinesFingerprint := flag.Bool("examines-fingerprint", false, "also let the account sign fingerprint decisions")
	examinesFacial := flag.Bool("examines-facial", false, "also let the account sign facial decisions")
	intelligence := flag.Bool("intelligence", false, "also make the account an intelligence analyst (every case and person, read-only)")
	coordinator := flag.Bool("coordinator", false, "also let the account distribute analyses and open comparisons")
	flag.Parse()

	if *matricula == "" {
		log.Fatal("-matricula is required")
	}

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	ctx := context.Background()
	repo := auth.NewRepository(sqlDB)

	existing, err := repo.FindByMatricula(ctx, *matricula)
	switch {
	case err == nil:
		user, err := repo.UpdateAccess(ctx, existing.Profile.ID, auth.Access{
			AccountRole:         auth.AccountRoleSuperAdmin,
			ExaminesFingerprint: existing.Profile.ExaminesFingerprint || *examinesFingerprint,
			ExaminesFacial:      existing.Profile.ExaminesFacial || *examinesFacial,
			IntelligenceAnalyst: existing.Profile.IntelligenceAnalyst || *intelligence,
			Coordinator:         existing.Profile.Coordinator || *coordinator,
			IsActive:            true,
		})
		cmdutil.Fatal(err)
		log.Printf("promoted %s (matricula %s) to %s", user.Username, user.Matricula, user.AccountRole)
		return
	case !errors.Is(err, sql.ErrNoRows):
		cmdutil.Fatal(err)
	}

	if *nome == "" || *ultimoNome == "" || *cargo == "" || *username == "" {
		log.Fatal("a new account needs -nome, -ultimo-nome, -cargo and -username")
	}
	password, err := readPassword()
	cmdutil.Fatal(err)
	user, err := repo.Create(ctx, auth.NewUser{
		Nome: *nome, UltimoNome: *ultimoNome, Matricula: *matricula, Cargo: *cargo, Username: *username, Password: password,
		Access: auth.Access{AccountRole: auth.AccountRoleSuperAdmin, ExaminesFingerprint: *examinesFingerprint, ExaminesFacial: *examinesFacial,
			IntelligenceAnalyst: *intelligence, Coordinator: *coordinator, IsActive: true},
	}, os.Getenv("EMAIL_DOMAIN"))
	cmdutil.Fatal(err)
	log.Printf("created %s %s (matricula %s, username %s)", user.AccountRole, user.Nome, user.Matricula, user.Username)
}

func readPassword() (string, error) {
	if password := os.Getenv("TRACKID_SUPERUSER_PASSWORD"); password != "" {
		return password, nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password: set TRACKID_SUPERUSER_PASSWORD or pipe it on standard input")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
