// create-superuser creates a SUPER_ADMIN account, the way a deployment gets its first one:
// there is no public sign-up, and every other account is created by an admin in the app
// (POST /api/admin/users). The password is read from TRACKID_SUPERUSER_PASSWORD or, when that
// is unset, from a hidden prompt when standard input is a terminal, or else from the first line
// of standard input, so it never appears in the shell history:
//
//	read -rs PW && echo "$PW" | create-superuser -first-name Ana -last-name Souza -staff-number 0001 -job-title Perita -username ana.souza
//
// With -staff-number naming an account that already exists, it promotes that account to
// SUPER_ADMIN (and reactivates it) instead, leaving its password as it is.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/internal/cmdutil"
	"github.com/rodfileto/trackid/internal/env"
	"golang.org/x/term"
)

func main() {
	env.Load()

	firstName := flag.String("first-name", "", "first name")
	lastName := flag.String("last-name", "", "last name")
	staffNumber := flag.String("staff-number", "", "the user's number in their organization, used to sign in (required)")
	jobTitle := flag.String("job-title", "", "job title")
	username := flag.String("username", "", "username, which names the user on every decision they sign and never changes")
	examinesFingerprint := flag.Bool("examines-fingerprint", false, "also let the account sign fingerprint decisions")
	examinesFacial := flag.Bool("examines-facial", false, "also let the account sign facial decisions")
	intelligence := flag.Bool("intelligence", false, "also make the account an intelligence analyst (every case and person, read-only)")
	coordinator := flag.Bool("coordinator", false, "also let the account distribute analyses and open comparisons")
	flag.Parse()

	if *staffNumber == "" {
		log.Fatal("-staff-number is required")
	}

	sqlDB := cmdutil.OpenDB()
	defer sqlDB.Close()
	ctx := context.Background()
	repo := auth.NewRepository(sqlDB)

	existing, err := repo.FindByStaffNumber(ctx, *staffNumber)
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
		log.Printf("promoted %s (staff number %s) to %s", user.Username, user.StaffNumber, user.AccountRole)
		return
	case !errors.Is(err, sql.ErrNoRows):
		cmdutil.Fatal(err)
	}

	if *firstName == "" || *lastName == "" || *jobTitle == "" || *username == "" {
		log.Fatal("a new account needs -first-name, -last-name, -job-title and -username")
	}
	password, err := readPassword()
	cmdutil.Fatal(err)
	user, err := repo.Create(ctx, auth.NewUser{
		FirstName: *firstName, LastName: *lastName, StaffNumber: *staffNumber, JobTitle: *jobTitle, Username: *username, Password: password,
		Access: auth.Access{AccountRole: auth.AccountRoleSuperAdmin, ExaminesFingerprint: *examinesFingerprint, ExaminesFacial: *examinesFacial,
			IntelligenceAnalyst: *intelligence, Coordinator: *coordinator, IsActive: true},
	}, os.Getenv("EMAIL_DOMAIN"))
	cmdutil.Fatal(err)
	log.Printf("created %s %s (staff number %s, username %s)", user.AccountRole, user.FirstName, user.StaffNumber, user.Username)
}

func readPassword() (string, error) {
	if password := os.Getenv("TRACKID_SUPERUSER_PASSWORD"); password != "" {
		return password, nil
	}
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		password, err := promptHidden(fd, "Enter password: ")
		if err != nil {
			return "", err
		}
		confirm, err := promptHidden(fd, "Confirm password: ")
		if err != nil {
			return "", err
		}
		if password == "" || password != confirm {
			return "", errors.New("passwords are empty or do not match")
		}
		return password, nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password: set TRACKID_SUPERUSER_PASSWORD or pipe it on standard input")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptHidden asks on standard error and reads a line from the terminal without echoing it.
func promptHidden(fd int, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
