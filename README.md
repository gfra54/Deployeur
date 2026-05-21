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
- [ ] `init` — scan, génération `.deployeur.yml`, enregistrement webhook
- [ ] `webhook` — daemon HMAC, TLS direct sur port dédié (pas de reverse proxy)
- [ ] `setup` — user, dossiers, service systemd, sudoers, port + TLS (cert existant ou certbot)
- [ ] `status`, `logs`

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
