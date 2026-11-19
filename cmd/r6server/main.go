// Command r6server is the shared server: spaces of matches, replays sent by
// their members, statistics served to the web interface.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdrillin/r6-stats/server"
)

const usage = `usage:
  r6server [-data dir] serve [-addr host:port] [-web dir]
                                       serve the API (and the built interface)
  r6server [-data dir] spaces          list the spaces
  r6server [-data dir] create <name> [team|league]
                                       create a space, print its id
  r6server [-data dir] import <space-id> <dir>...
                                       send every .rec file under the folders

-data is the folder of the database and the raw replays (default r6-data).
There is no authentication yet: serve only listens on the local machine.
`

func main() {
	log.SetFlags(0)
	fl := flag.NewFlagSet("r6server", flag.ExitOnError)
	fl.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	data := fl.String("data", "r6-data", "folder of the database and raw replays")
	fl.Parse(os.Args[1:])
	args := fl.Args()
	if len(args) == 0 {
		fl.Usage()
		os.Exit(2)
	}

	s, err := server.Open(*data)
	if err != nil {
		log.Fatal("r6server: ", err)
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
	default:
		fl.Usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal("r6server: ", err)
	}
}

func serve(s *server.Server, args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fl.String("addr", "127.0.0.1:8080", "address to listen on")
	web := fl.String("web", "", "folder of the built web interface")
	fl.Parse(args)

	// Without accounts, anyone reaching the server could read and write
	// every space: refuse anything but the local machine.
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("refusing to listen on %s: there is no authentication yet, use 127.0.0.1", host)
	}

	if n, err := s.Redecode(); err != nil {
		return err
	} else if n > 0 {
		log.Printf("%d rounds decoded again with the current parser", n)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", s.Handler())
	if *web != "" {
		mux.Handle("/", http.FileServer(spaFS{http.Dir(*web)}))
	}
	log.Printf("listening on http://%s", *addr)
	return http.ListenAndServe(*addr, mux)
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

func create(s *server.Server, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	kind := server.Team
	if len(args) == 2 {
		kind = server.SpaceKind(args[1])
	}
	sp, err := s.CreateSpace(args[0], kind)
	if err != nil {
		return err
	}
	fmt.Println(sp.ID)
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
	fmt.Printf("%d added, %d already in the space, %d invalid\n",
		counts[server.Added], counts[server.Already], counts[server.Invalid])
	return nil
}
