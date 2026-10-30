// Command authspike signs in to Apple Music using the web player developer
// token, validates the Music User Token, and optionally saves it to an env
// file. By default it runs the loopback redirect flow (docs/auth.md §4) in
// a browser; -native runs Sona's own Apple Account sign-in (§4.7) on the
// terminal: account, password (not echoed) and two-factor code.
//
// Usage: authspike [-native] [-open=false] [-save .env]
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
)

func main() {
	native := flag.Bool("native", false, "sign in with Apple Account name, password and code on this terminal")
	open := flag.Bool("open", true, "open the start URL with xdg-open (false: print it only)")
	save := flag.String("save", "", "write MUSIC_USER_TOKEN=<token> to this file (mode 0600)")
	timeout := flag.Duration("timeout", 10*time.Minute, "sign-in deadline")
	flag.Parse()
	if err := run(*native, *open, *save, *timeout); err != nil {
		slog.Error("authspike failed", "err", err)
		os.Exit(1)
	}
}

func run(native, open bool, save string, timeout time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dev := devtoken.NewWebPlayer(nil)
	tok, err := dev.Token(ctx)
	if err != nil {
		return err
	}
	fmt.Println("dev token:", tok)

	var res mut.Result
	if native {
		r, err := nativeSignIn(ctx, tok.JWT)
		if err != nil {
			return err
		}
		res = mut.Result{MusicUserToken: r.MusicUserToken, Restricted: r.Restricted}
	} else if res, err = loopbackSignIn(ctx, tok.JWT, open, timeout); err != nil {
		return err
	}
	fmt.Println("received:", res)

	api := applemusic.New(nil, dev, func() string { return res.MusicUserToken })
	sf, err := api.Storefront(ctx)
	if err != nil {
		return fmt.Errorf("validate MUT: %w", err)
	}
	fmt.Printf("signed in: storefront=%s (%s) %s\n", sf.ID, sf.Attributes.Name, res)

	if save != "" {
		if err := writeEnv(save, "MUSIC_USER_TOKEN", res.MusicUserToken); err != nil {
			return err
		}
		fmt.Println("saved to", save)
	}
	return nil
}

func loopbackSignIn(ctx context.Context, devToken string, open bool, timeout time.Duration) (mut.Result, error) {
	sess, err := mut.Start(devToken, mut.Options{Timeout: timeout})
	if err != nil {
		return mut.Result{}, err
	}
	fmt.Println("open to sign in:", sess.StartURL())
	if open {
		if err := exec.Command("xdg-open", sess.StartURL()).Start(); err != nil {
			fmt.Println("xdg-open failed; open the URL manually:", err)
		}
	}
	return sess.Wait(ctx)
}

func nativeSignIn(ctx context.Context, devToken string) (appleid.Result, error) {
	in := bufio.NewReader(os.Stdin)
	line := func(prompt string) (string, error) {
		fmt.Print(prompt)
		s, err := in.ReadString('\n')
		return strings.TrimSpace(s), err
	}
	account, err := line("Apple Account: ")
	if err != nil {
		return appleid.Result{}, err
	}
	var pw []byte
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		fmt.Print("Password: ")
		pw, err = term.ReadPassword(fd)
		fmt.Println()
	} else {
		var s string
		s, err = line("Password: ")
		pw = []byte(s)
	}
	if err != nil {
		return appleid.Result{}, err
	}

	sess, err := appleid.Begin(ctx, devToken, appleid.Options{})
	if err != nil {
		return appleid.Result{}, err
	}
	defer sess.Close()
	sf, err := sess.SignIn(ctx, account, string(pw))
	clear(pw)
	if err != nil {
		return appleid.Result{}, err
	}
	if sf != nil {
		phone, mode := sf.SentToPhone, ""
		prompt := fmt.Sprintf("Code (%d digits): ", sf.CodeLength)
		if sf.TrustedDevices {
			fmt.Println("Apple prompted your trusted devices; enter the code shown there, or get one by text.")
		} else {
			fmt.Println("No Apple device on this account; codes come by text or call.")
		}
		for _, p := range sf.Phones {
			fmt.Printf("  phone %d: %s (%s)\n", p.ID, p.Number, p.Mode)
		}
		if len(sf.Phones) > 0 {
			prompt = fmt.Sprintf("Code (%d digits), or sms:<phone id>: ", sf.CodeLength)
		}
		for {
			c, err := line(prompt)
			if err != nil {
				return appleid.Result{}, err
			}
			if id, ok := strings.CutPrefix(c, "sms:"); ok {
				n, _ := strconv.Atoi(id)
				if err := sess.SendCodeToPhone(ctx, n, "sms"); err != nil {
					fmt.Println(err)
					continue
				}
				phone, mode = n, "sms"
				continue
			}
			err = sess.VerifyCode(ctx, c, phone, mode)
			if errors.Is(err, appleid.ErrIncorrectCode) {
				fmt.Println(err)
				continue
			}
			if err != nil {
				return appleid.Result{}, err
			}
			break
		}
	}
	return sess.MusicUserToken(ctx)
}

// writeEnv sets key in an env file, preserving other lines, with mode 0600.
func writeEnv(path, key, value string) error {
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if k, _, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == key {
				continue
			}
			if l != "" {
				lines = append(lines, l)
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	lines = append(lines, key+"="+value)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
