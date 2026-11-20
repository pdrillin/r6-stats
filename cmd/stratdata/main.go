// Command stratdata is the shared server: spaces of matches, replays sent by
// their members, statistics served to the web interface.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/pdrillin/stratdata/server"
)

const usage = `usage:
  stratdata [-data dir] serve [-addr host:port] [-url base-url] [-web dir] [-dev-login]
                                       serve the API (and the built interface)
  stratdata [-data dir] spaces          list the spaces
  stratdata [-data dir] create <name> [team|league]
                                       create a space, print its id
  stratdata [-data dir] invite <space-id> [manager|player]
                                       print an invitation link to the space
  stratdata [-data dir] delete <space-id>
                                       delete a space, its matches and invitations
                                       (asks for its name; raw replays no other
                                       space holds are deleted too)
  stratdata [-data dir] import <space-id> <dir>...
                                       send every .rec file under the folders
  stratdata [-data dir] backup <file>   write a consistent copy of the database
                                       (safe while serve runs)

-data is the folder of the database and the raw replays (default stratdata-data).

The server reads the environment, completed by a .env file in the current
folder (model: .env.example; variables already set win):
  STRATDATA_URL   default of -url
  STRATDATA_DISCORD_CLIENT_ID, STRATDATA_DISCORD_CLIENT_SECRET   the OAuth2 application
  STRATDATA_CREATORS   comma-separated Discord ids allowed to create spaces
                (closed beta; empty lets everyone signed in create spaces)
  STRATDATA_OPERATOR, STRATDATA_CONTACT   who runs the site and how to reach them
                (shown on the privacy page)
Sign-in with Discord needs the application; it must list <base-url>/auth/callback as redirect.

-dev-login signs in by name, without Discord, at /auth/dev?name=…: only on
the local machine. Off the local machine, serve requires an https -url.
`

func main() {
	log.SetFlags(0)
	if err := loadEnvFile(".env"); err != nil {
		log.Fatal("stratdata: ", err)
	}
	fl := flag.NewFlagSet("stratdata", flag.ExitOnError)
	fl.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	data := fl.String("data", defaultData, "folder of the database and raw replays")
	fl.Parse(os.Args[1:])
	if *data == defaultData {
		*data = dataFolder()
	}
	args := fl.Args()
	if len(args) == 0 {
		fl.Usage()
		os.Exit(2)
	}

	s, err := server.Open(*data)
	if err != nil {
		log.Fatal("stratdata: ", err)
	}
	defer s.Close()

	switch args[0] {
	case "serve":
		err = serve(s, args[1:])
	case "spaces":
		err = spaces(s)
	case "create":
		err = create(s, args[1:])
	case "import":
		err = importDirs(s, args[1:])
	case "invite":
		err = invite(s, args[1:])
	case "delete":
		err = deleteSpace(s, args[1:], os.Stdin)
	case "backup":
		if len(args) != 2 {
			fl.Usage()
			os.Exit(2)
		}
		err = s.Backup(args[1])
	default:
		fl.Usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal("stratdata: ", err)
	}
}

func serve(s *server.Server, args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fl.String("addr", "127.0.0.1:8642", "address to listen on")
	base := fl.String("url", env("URL"), "address users reach the server at (default STRATDATA_URL, else http://<addr>)")
	web := fl.String("web", "", "folder of the built web interface")
	dev := fl.Bool("dev-login", false, "sign in by name without Discord (local machine only)")
	fl.Parse(args)

	cfg := server.Config{
		BaseURL: strings.TrimSuffix(*base, "/"),
		Discord: server.Discord{
			ClientID:     env("DISCORD_CLIENT_ID"),
			ClientSecret: env("DISCORD_CLIENT_SECRET"),
		},
		DevLogin: *dev,
		Operator: env("OPERATOR"),
		Contact:  env("CONTACT"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://" + *addr
	}
	for _, id := range strings.Split(env("CREATORS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			cfg.Creators = append(cfg.Creators, id)
		}
	}

	// Off the local machine, sessions must travel over HTTPS and the
	// development sign-in, which trusts any name, must be off.
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		if *dev {
			return fmt.Errorf("-dev-login only works on the local machine")
		}
		if !strings.HasPrefix(cfg.BaseURL, "https://") {
			return fmt.Errorf("listening on %s needs an https -url (put the server behind an HTTPS proxy)", host)
		}
	}
	if cfg.Discord.ClientID == "" && !*dev {
		log.Printf("warning: STRATDATA_DISCORD_CLIENT_ID is not set, nobody can sign in")
	}
	if cfg.Discord.ClientID != "" && cfg.Discord.ClientSecret == "" {
		return fmt.Errorf("STRATDATA_DISCORD_CLIENT_SECRET is not set")
	}
	if cfg.Contact == "" {
		log.Printf("warning: STRATDATA_CONTACT is not set, the privacy page shows no contact")
	}
	if len(cfg.Creators) == 0 {
		log.Printf("STRATDATA_CREATORS is empty: everyone signed in can create spaces")
	}

	if n, err := s.Redecode(); err != nil {
		return err
	} else if n > 0 {
		log.Printf("%d rounds decoded again with the current parser", n)
	}

	mux := http.NewServeMux()
	// For monitoring: answers 200 while the database does.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Ping(); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	h := s.Handler(cfg)
	mux.Handle("/api/", h)
	mux.Handle("/auth/", h)
	if *web == "" {
		// Somewhere to land after signing in, until the interface is
		// served.
		mux.Handle("/", s.Home())
	} else {
		mux.Handle("/", http.FileServer(spaFS{http.Dir(*web)}))
	}
	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
		// A client must send its headers quickly; bodies may take long
		// (a match is tens of megabytes on a slow upload).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	// systemd stops the service with SIGTERM: finish the requests in
	// progress (an upload being decoded) before the database closes.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	done := make(chan error, 1)
	go func() {
		<-stop
		log.Printf("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()
	// Bind before announcing the address, so a port already in use is the
	// only message the user sees.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s (%s)", ln.Addr(), cfg.BaseURL)
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return <-done
}

// loadEnvFile sets the variables of a KEY=value file (comments with #,
// optional quotes) that the environment does not set already: the local
// settings, secret included, stay out of the shell history. A missing file
// is fine.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return fmt.Errorf("%s:%d: expected KEY=value", path, n+1)
		}
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return nil
}

const defaultData = "stratdata-data"

// dataFolder is the default data folder, or the one of the project's former
// name when only that one exists.
func dataFolder() string {
	if _, err := os.Stat(defaultData); os.IsNotExist(err) {
		if fi, err := os.Stat("r6-data"); err == nil && fi.IsDir() {
			log.Printf("using r6-data, the data folder of the former name (rename it to %s)", defaultData)
			return "r6-data"
		}
	}
	return defaultData
}

// env reads STRATDATA_<name>, or R6_<name> as the project's former name set
// it.
func env(name string) string {
	if v := os.Getenv("STRATDATA_" + name); v != "" {
		return v
	}
	if v := os.Getenv("R6_" + name); v != "" {
		log.Printf("R6_%s is read for STRATDATA_%s: rename it", name, name)
		return v
	}
	return ""
}

// spaFS serves index.html for paths that are not files, so the interface's
// own routes load the application.
type spaFS struct{ http.FileSystem }

func (f spaFS) Open(name string) (http.File, error) {
	file, err := f.FileSystem.Open(name)
	if err != nil {
		return f.FileSystem.Open("/index.html")
	}
	return file, nil
}

func spaces(s *server.Server) error {
	list, err := s.Spaces()
	if err != nil {
		return err
	}
	for _, sp := range list {
		fmt.Printf("%s  %-6s  %4d replays  %s\n", sp.ID, sp.Kind, sp.Files, sp.Name)
	}
	return nil
}

// deleteSpace deletes a space once its name is typed back, as the web
// interface asks: an id copied from the wrong line costs a whole team its
// history.
func deleteSpace(s *server.Server, args []string, in io.Reader) error {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	list, err := s.Spaces()
	if err != nil {
		return err
	}
	var sp *server.Space
	for i := range list {
		if list[i].ID == args[0] {
			sp = &list[i]
		}
	}
	if sp == nil {
		return fmt.Errorf("no space %q", args[0])
	}
	fmt.Fprintf(os.Stderr, "Space %q (%s, %d replays) will be deleted for all its members.\nType its name to confirm: ", sp.Name, sp.Kind, sp.Files)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != sp.Name {
		return fmt.Errorf("name does not match, nothing deleted")
	}
	if err := s.DeleteSpace(sp.ID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "deleted %s\n", sp.Name)
	return nil
}

func create(s *server.Server, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	kind := server.Team
	if len(args) == 2 {
		kind = server.SpaceKind(args[1])
	}
	sp, err := s.CreateSpace(args[0], kind, "")
	if err != nil {
		return err
	}
	fmt.Println(sp.ID)
	return nil
}

// invite prints an invitation link: the way to give a space created from the
// command line its first manager.
func invite(s *server.Server, args []string) error {
	fl := flag.NewFlagSet("invite", flag.ExitOnError)
	base := fl.String("url", "http://127.0.0.1:8642", "address users reach the server at")
	fl.Parse(args)
	args = fl.Args()
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	role := server.Manager
	if len(args) == 2 {
		role = server.Role(args[1])
	}
	known := false
	list, err := s.Spaces()
	if err != nil {
		return err
	}
	for _, sp := range list {
		known = known || sp.ID == args[0]
	}
	if !known {
		return fmt.Errorf("no space %s", args[0])
	}
	inv, err := s.Invite(args[0], "", role)
	if err != nil {
		return err
	}
	fmt.Printf("%s/invite/%s\n(%s, valid until %s)\n", strings.TrimSuffix(*base, "/"), inv.Token, role, inv.ExpiresAt)
	return nil
}

func importDirs(s *server.Server, args []string) error {
	if len(args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	id := args[0]
	var paths []string
	for _, root := range args[1:] {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".rec") {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.Strings(paths)
	counts := map[server.Status]int{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		res, err := s.Add(id, server.Upload{Name: filepath.Base(p), Data: data})
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		counts[res.Status]++
		if res.Status == server.Invalid {
			log.Printf("%s: %s", p, res.Error)
		}
	}
	fmt.Printf("%d added, %d already in the space, %d ignored (not custom games), %d invalid\n",
		counts[server.Added], counts[server.Already], counts[server.Ignored], counts[server.Invalid])
	return nil
}
