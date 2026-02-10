# Shopify Status RSS

A small server that scrapes the Shopify status page and generates an RSS feed, since they don't have one.

![Screen shot of an RSS reader showing the Shopify RSS feed](./screenshot-1.png)

## 🚀 Getting Started

This service is meant to be self-hosted. The easiest way to get started is by using a Docker compose file. The following example compose file will start a Postgres database and the Shopify Status RSS server. Before you set up the compose file, create a file named `.env`. Here is a sample `.env` file to get you started.

```
HOST="0.0.0.0:3000"
DSN="postgres://shopifystatus:password@postgres:5432/shopify_status_rss"
CRON_SCHEDULE="*/30 * * * *"
LOG_LEVEL="info"
STATUS_PAGE_URL="https://my.shopifystatus.com"

POSTGRES_USER=shopifystatus
POSTGRES_PASSWORD=password
POSTGRES_DB=shopify_status_rss
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
```

Now, the Docker compose file. 

```yaml 
services:
  postgres:
    image: postgres:17.7
    container_name: shopify-status-rss-postgres
    restart: unless-stopped
    ports:
      - "5432:5432"
    env_file:
      - ./.env
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-postgres}"]
      interval: 30s
      timeout: 10s
      retries: 3
  shopify-status-rss:
    build:
      image: adampresley/shopify-status-rss
    env_file:
      - ./.env
    ports:
      - 3000:3000
    depends_on:
      - postgres

volumes:
  postgres_data:
```

> The above environment variables and Docker compose file are **not** suitable for a production deployment. It exposes the Postgres database port, and has weak credentials. DO NOT DEPLOY THIS TO PRODUCTION WIT THESE SETTINGS! You've been warned.

Now, test it out! Visit http://localhost:3000/status.rss to see the RSS feed.

![Screen shot of the RSS feed](./screenshot-2.png)

