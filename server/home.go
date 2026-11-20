package server

import (
	"html/template"
	"net/http"
)

// homePage is shown at / when the web interface is not served: who is signed
// in, their spaces, and how to sign in or out. Without it, coming back from
// Discord would land on a 404.
var homePage = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Stratdata</title>
<style>
:root { color-scheme: light dark; font-family: system-ui, sans-serif; }
body { max-width: 40rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.5; }
code { background: rgba(127,127,127,.15); padding: 0 .3em; border-radius: 4px; }
button, .button { font: inherit; padding: .4rem .9rem; border-radius: 6px; border: 1px solid #888;
  background: none; color: inherit; cursor: pointer; text-decoration: none; display: inline-block; }
li { margin: .3rem 0; }
.muted { opacity: .7; }
</style>
</head>
<body>
<h1>Stratdata</h1>
{{if .User}}
<p>Connecté en tant que <b>{{.User.Name}}</b>.</p>
{{if .Spaces}}
<h2>Tes espaces</h2>
<ul>
{{range .Spaces}}<li><b>{{.Name}}</b> <span class="muted">· {{if eq .Kind "league"}}ligue{{else}}équipe{{end}} · {{if eq .Role "manager"}}responsable{{else}}joueur{{end}} · {{.Files}} replays · <code>{{.ID}}</code></span></li>
{{end}}
</ul>
{{else}}
<p class="muted">Tu n'es membre d'aucun espace. Rejoins-en un avec un lien d'invitation{{if .CanCreate}}, ou crée le tien{{end}}.</p>
{{end}}
<p class="muted">L'interface web n'est pas encore branchée sur ce serveur : pour l'instant, les espaces se gèrent par l'API.</p>
<button id="logout">Se déconnecter</button>
<script>
document.getElementById("logout").onclick = async () => {
  await fetch("/auth/logout", { method: "POST" });
  location.reload();
};
</script>
{{else}}
<p>Tu n'es pas connecté.</p>
<p><a class="button" href="/auth/login">Se connecter avec Discord</a></p>
{{end}}
</body>
</html>
`))

// Home serves homePage.
func (s *Server) Home() http.Handler {
	return s.withUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := struct {
			User      *User
			Spaces    []Space
			CanCreate bool
		}{}
		if u, ok := currentUser(r); ok {
			data.User = &u
			data.CanCreate = s.canCreate(u)
			spaces, err := s.SpacesOf(u.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			data.Spaces = spaces
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		homePage.Execute(w, data)
	}))
}
