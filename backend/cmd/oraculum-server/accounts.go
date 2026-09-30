package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/accountsdb"
	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// accountsFlag は、利用者のアカウントとセッションを置く file を渡す flag である。
//
// 渡した起動は、`/api/` の要求にログインしたセッションを求め、分析者の判断の著者をログイン名に
// する。loopback 以外の --addr で待ち受ける起動は、この flag を必要とする。
const accountsFlag = "--accounts"

// accountsCommand は、アカウントを管理する subcommand の名前である。
const accountsCommand = "accounts"

const accountsUsage = "usage: oraculum-server accounts <add|passwd|disable|enable> " + accountsFlag +
	" <file> <login> [--display-name <name>]\n" +
	"add and passwd read the password from the first line of the standard input."

// accountsPort は、アカウントの file を api の port にする。file の失敗を api の失敗に写す。
type accountsPort struct{ db *accountsdb.DB }

func (p accountsPort) Login(ctx context.Context, login, password string) (api.AccountSession, error) {
	session, err := p.db.Login(ctx, login, password)
	if errors.Is(err, accountsdb.ErrLoginRejected) {
		return api.AccountSession{}, api.ErrLoginRejected
	}
	if err != nil {
		return api.AccountSession{}, err
	}
	return api.AccountSession{Token: session.Token, ExpiresAt: session.ExpiresAt,
		Account: api.Account{Login: session.Account.Login, DisplayName: session.Account.DisplayName}}, nil
}

func (p accountsPort) SessionAccount(ctx context.Context, token string) (api.Account, error) {
	account, err := p.db.SessionAccount(ctx, token)
	if errors.Is(err, accountsdb.ErrSessionInvalid) {
		return api.Account{}, api.ErrSessionInvalid
	}
	if err != nil {
		return api.Account{}, err
	}
	return api.Account{Login: account.Login, DisplayName: account.DisplayName}, nil
}

func (p accountsPort) Logout(ctx context.Context, token string) error { return p.db.Logout(ctx, token) }

func (p accountsPort) Account(ctx context.Context, login string) (api.Account, error) {
	account, err := p.db.Account(ctx, login)
	if errors.Is(err, accountsdb.ErrAccountNotFound) {
		return api.Account{}, api.ErrAccountNotFound
	}
	if err != nil {
		return api.Account{}, err
	}
	return api.Account{Login: account.Login, DisplayName: account.DisplayName}, nil
}

// runAccounts はアカウントを管理する subcommand を実行する。
//
// **パスワードを引数で受け取らない。** 引数は process の一覧と shell の履歴に残る。
func runAccounts(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	action, path, login, displayName, err := parseAccountsArgs(args)
	if err != nil {
		return reportUsageError(stderr, err, accountsUsage)
	}
	ctx := context.Background()
	open := accountsdb.Open
	if action == "add" {
		open = accountsdb.Create
	}
	db, err := open(ctx, path)
	if err != nil {
		return reportError(stderr, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			reportError(stderr, err)
		}
	}()
	switch action {
	case "add", "passwd":
		password, err := readPassword(stdin)
		if err != nil {
			return reportError(stderr, err)
		}
		if action == "add" {
			if displayName == "" {
				displayName = login
			}
			err = db.AddAccount(ctx, login, displayName, password)
		} else {
			err = db.SetPassword(ctx, login, password)
		}
		if err != nil {
			return reportError(stderr, err)
		}
	case "disable", "enable":
		if err := db.SetDisabled(ctx, login, action == "disable"); err != nil {
			return reportError(stderr, err)
		}
	}
	if _, err := output.Fprintf(stdout, "%s %s\n", action, login); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func parseAccountsArgs(args []string) (action, path, login, displayName string, err error) {
	if len(args) == 0 {
		return "", "", "", "", errors.New("parsing arguments: the action is required")
	}
	action = args[0]
	switch action {
	case "add", "passwd", "disable", "enable":
	default:
		return "", "", "", "", fmt.Errorf("parsing arguments: unknown action %q", action)
	}
	rest := args[1:]
	for index := 0; index < len(rest); index++ {
		switch flag := rest[index]; flag {
		case accountsFlag, "--display-name":
			index++
			if index == len(rest) || rest[index] == "" {
				return "", "", "", "", fmt.Errorf("parsing arguments: %s requires a value", flag)
			}
			if flag == accountsFlag {
				path = rest[index]
			} else if action == "add" {
				displayName = rest[index]
			} else {
				return "", "", "", "", errors.New("parsing arguments: --display-name is only for add")
			}
		default:
			if login != "" || strings.HasPrefix(flag, "--") {
				return "", "", "", "", fmt.Errorf("parsing arguments: unexpected argument %q", flag)
			}
			login = flag
		}
	}
	if path == "" || login == "" {
		return "", "", "", "", fmt.Errorf("parsing arguments: %s and the login are required", accountsFlag)
	}
	return action, path, login, displayName, nil
}

// readPassword は標準入力の最初の行をパスワードとして読む。行末の改行を含めない。
func readPassword(stdin io.Reader) (string, error) {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading the password from the standard input: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("reading the password from the standard input: the first line is empty")
	}
	return password, nil
}
