FROM node:24-alpine AS build

ARG VITE_API_BASE_URL=http://localhost:8080/api
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

WORKDIR /src
COPY frontend/package*.json ./
RUN npm install

COPY frontend/ ./
# Downloadable artifacts are no longer baked into the image; they are served
# from a mounted volume (dev) or Azure Blob (prod). See artifacts/README.md.
RUN npm run build

FROM nginx:1.27-alpine

COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html

EXPOSE 80
