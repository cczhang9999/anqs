# ANQS

This repository contains the ANQS frontend and backend as separate projects:

- `xops-front-aegis-guard-master/` — Vue 3 + TypeScript frontend
- `xops-aegis-guard-master/` — Go API service

Each project keeps its own dependency files, Dockerfiles, and deployment manifests. Local credentials and environment-specific settings are excluded from Git.

## Configuration

- Backend: copy `xops-aegis-guard-master/config/config.example.yml` to `config/config.yml`, then set the database and cloud credentials for your environment.
- Frontend: copy `xops-front-aegis-guard-master/.env.example` to the environment file you use (for example `.env.base`) and set the API and SSO endpoints.

See each project's README for development and deployment instructions.
