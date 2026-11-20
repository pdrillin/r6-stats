import { useApi } from "../api";

interface Info {
  operator: string;
  contact: string;
}

/**
 * What the site keeps, why, for how long, and the user's rights (GDPR).
 * Keep it true: any change to what the server stores or for how long must be
 * reflected here.
 */
export function PrivacyPage() {
  const info = useApi<Info>("/info");
  const operator = info.data?.operator || "l'exploitant du site";
  const contact = info.data?.contact;
  return (
    <main className="narrow prose">
      <h1>Confidentialité</h1>
      <p className="muted">Dernière mise à jour : novembre 2026.</p>

      <h2>Qui est responsable</h2>
      <p>
        Le site est exploité par <b>{operator}</b>, responsable du traitement des données au sens du RGPD.{" "}
        {contact ? (
          <>
            Pour toute question ou demande : <a href={`mailto:${contact}`}>{contact}</a>.
          </>
        ) : (
          <span className="error">L'adresse de contact n'a pas encore été renseignée.</span>
        )}
      </p>

      <h2>Ce qui est conservé, et pourquoi</h2>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th className="text">Données</th>
              <th className="text">Pourquoi</th>
              <th className="text">Combien de temps</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td className="text">
                Ton compte : identifiant, pseudo et avatar Discord (rien d'autre : ni email, ni serveurs, ni messages)
              </td>
              <td className="text">Te connecter et t'afficher aux membres de tes espaces</td>
              <td className="text">Jusqu'à la suppression de ton compte</td>
            </tr>
            <tr>
              <td className="text">Tes espaces, ton rôle, les invitations que tu crées</td>
              <td className="text">Faire fonctionner les espaces privés</td>
              <td className="text">Jusqu'à la suppression de l'espace ou de ton compte ; une invitation expire après 7 jours</td>
            </tr>
            <tr>
              <td className="text">
                Les replays envoyés et ce qui en est extrait : pseudos des 10 joueurs du match, opérateurs, kills,
                positions par pièce, scores. Seules les parties personnalisées (praccs, matchs officiels) sont gardées :
                un replay de ranked, de partie rapide ou d'arcade est écarté à l'envoi, sans être stocké
              </td>
              <td className="text">Calculer les statistiques de l'espace</td>
              <td className="text">
                Jusqu'à la suppression du match ou de l'espace. Le fichier de replay brut peut être supprimé plus tôt :
                les statistiques n'en ont pas besoin
              </td>
            </tr>
            <tr>
              <td className="text">Un cookie de session, et un cookie de quelques minutes pendant la connexion</td>
              <td className="text">Rester connecté</td>
              <td className="text">30 jours, ou jusqu'à la déconnexion</td>
            </tr>
            <tr>
              <td className="text">Les sauvegardes de tout ce qui précède</td>
              <td className="text">Pouvoir tout restaurer après une panne</td>
              <td className="text">12 mois au plus : une donnée supprimée disparaît des sauvegardes au plus tard 12 mois après</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p>
        Il n'y a ni publicité, ni mesure d'audience, ni cookie de suivi : les seuls cookies servent à la connexion, ils
        ne demandent donc pas de consentement.
      </p>

      <h2>Sur quelle base</h2>
      <ul>
        <li>
          <b>Ton compte et tes espaces</b> : ils sont nécessaires au service que tu demandes en t'inscrivant.
        </li>
        <li>
          <b>Les données de jeu des autres joueurs</b> présents dans un replay, adversaires compris : l'intérêt légitime
          des équipes à analyser leurs propres matchs. Ces données restent dans l'espace privé qui les a reçues : elles
          ne sont jamais publiées ni réunies dans un profil public, et un adversaire ne voit rien de l'espace d'une
          équipe qu'il a affrontée.
        </li>
      </ul>

      <h2>Qui y a accès</h2>
      <ul>
        <li>Les membres d'un espace, et eux seuls, voient ses matchs et ses statistiques.</li>
        <li>
          L'exploitant du site, pour le faire fonctionner. L'hébergeur du serveur stocke les données pour son compte,
          sans les exploiter.
        </li>
        <li>
          Discord, pour la connexion. Les avatars sont affichés depuis les serveurs de Discord, qui voient donc ton
          adresse IP quand tu ouvres la page des membres.
        </li>
      </ul>
      <p>Aucune donnée n'est vendue ni transmise à d'autres.</p>

      <h2>Tes droits</h2>
      <ul>
        <li>
          <b>Accès et portabilité</b> : télécharge tout ce qui est gardé sur toi depuis « Mon compte », sur la page
          d'accueil.
        </li>
        <li>
          <b>Rectification</b> : ton pseudo et ton avatar sont repris de Discord à chaque connexion.
        </li>
        <li>
          <b>Effacement</b> : supprime ton compte depuis « Mon compte ». Un responsable d'espace peut supprimer un match
          ou tout l'espace.
        </li>
        <li>
          <b>Opposition, et données de joueurs sans compte</b> : si tu apparais dans un replay envoyé par une autre
          équipe et souhaites le retrait de tes données, écris {contact ? <a href={`mailto:${contact}`}>{contact}</a> : "à l'exploitant"}.
        </li>
      </ul>
      <p>
        Si tu estimes que tes droits ne sont pas respectés, tu peux saisir la CNIL :{" "}
        <a href="https://www.cnil.fr/fr/plaintes" rel="noreferrer">www.cnil.fr/fr/plaintes</a>.
      </p>

      <p>
        <a href="/">Retour à l'accueil</a>
      </p>
    </main>
  );
}
