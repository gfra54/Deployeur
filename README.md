# deployeur

Outil CLI de déploiement auto-hébergé pour serveurs LAMP / Node+PM2.
Un seul binaire, aucune dépendance runtime sur les serveurs.

## Build

```bash
go build -o deployeur .
# pour les serveurs (Linux amd64) :
GOOS=linux GOARCH=amd64 go build -o deployeur .
```

## État

- [x] `deploy` — fetch + ff-only (non destructif) + before/steps/after/on_failure, variables d'env
- [x] `init` — scan, génération `.deployeur.yml`, enregistrement webhook
- [x] `webhook` — daemon HMAC + coalescing, TLS direct sur port dédié (pas de reverse proxy)
- [x] `setup` — dossiers, service systemd, sudoers sous un user **existant** (--user / $SUDO_USER), port + TLS (cert existant ou certbot)
- [x] `status`, `logs`

## Architecture webhook

Le daemon écoute en TLS directement sur un port dédié (aléatoire, persisté dans
`/etc/deployeur/config.yml`), **sans reverse proxy**. URL annoncée par `init` :
`https://<hostname>:<port>/hooks/<repo>`. TLS : cert renseigné en config, sinon
cert Let's Encrypt du hostname détecté automatiquement. `/status` et `/healthz`
restent sur `127.0.0.1:9000` (local only).

## .deployeur.yml

```yaml
branch: master
before:
  - php maintenance.php on
steps:
  - composer install --no-dev --optimize-autoloader
  - npm ci
  - npm run build
after:
  - pm2 reload monapp
  - php maintenance.php off
on_failure:
  - php maintenance.php off
```

Mise à jour du code implicite avant `before`, **non destructive** :
`git fetch` puis `git merge --ff-only`. Si l'arbre de travail a des modifs locales
ou si l'historique a divergé, le déploiement s'interrompt sans rien écraser (exit 1,
erreur loguée + enregistrée dans le state).
Si le fichier est absent, les étapes sont auto-détectées (composer, npm build, artisan, pm2, wp).

Variables disponibles dans les commandes : `$REPO`, `$COMMIT`, `$BRANCH`, `$DEPLOY_DIR`.
