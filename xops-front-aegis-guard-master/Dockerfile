FROM harbor.zfit888.com/ywcw-base/node:22.5.1 as build-stage

WORKDIR /var/www/xops-front-aegis-guard/

COPY package.json pnpm-lock.yaml ./

RUN npm install -g pnpm@9.15.9 && pnpm install --frozen-lockfile

COPY . .

RUN pnpm run build:pro

FROM harbor.zfit888.com/ywdev-base/ywdev-nginx as production-stage

COPY --from=build-stage /var/www/xops-front-aegis-guard/dist /var/www/xops-front-aegis-guard/dist

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
