terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

data "cloudflare_zone" "freehire" {
  name = var.zone_name
}

# ==================== DNS ====================
# Proxied (orange cloud): traffic reaches the origin through Cloudflare, which is
# the whole point — an unproxied record is DNS only and caches nothing.

resource "cloudflare_record" "root" {
  zone_id = data.cloudflare_zone.freehire.id
  name    = "@"
  content = var.origin_server_ip
  type    = "A"
  proxied = true
  comment = "Hetzner origin — nginx in the web container"
}

resource "cloudflare_record" "www" {
  zone_id = data.cloudflare_zone.freehire.id
  name    = "www"
  content = var.zone_name
  type    = "CNAME"
  proxied = true
}

# The logo proxy the SPA and OG cards resolve company marks through. Its own
# subdomain so a long image cache never applies to page HTML.
resource "cloudflare_record" "logo" {
  zone_id = data.cloudflare_zone.freehire.id
  name    = "logo"
  content = var.origin_server_ip
  type    = "A"
  proxied = true
}

# ==================== Zone settings ====================

resource "cloudflare_zone_settings_override" "freehire" {
  zone_id = data.cloudflare_zone.freehire.id

  settings {
    # "strict" (not "full"): the origin serves a valid, publicly-trusted
    # certificate today, so Cloudflare can verify it rather than accept any cert.
    ssl              = "strict"
    always_use_https = "on"
    # HTTP/3 and 0-RTT cut round trips for visitors far from Helsinki, which is
    # everyone outside northern Europe — the origin is a single box there.
    http3    = "on"
    zero_rtt = "on"
    brotli   = "on"
    # Deliberately NOT raised beyond "essentially_off"/"low": this site wants to be
    # crawled. A challenge page served to a crawler is an unindexed page, and the
    # public API is consumed by scripts and agents that cannot solve one.
    security_level = "essentially_off"
  }
}

# ==================== Cache ====================
# The origin already states its own policy per response (web/src/lib/httpCache.ts):
# anonymous public HTML is `public, s-maxage=300, stale-while-revalidate=86400`,
# and anything tied to a session is `private, no-store`. These rules make HTML
# eligible for the edge cache and then RESPECT those headers, so the decision has
# one home — the application — instead of being restated (and eventually
# contradicted) here.
#
# There is deliberately no bot-blocking ruleset, unlike the sibling telagon config.
# freehire publishes an HTTP API, a CLI and a ChatGPT action: its legitimate
# clients ARE `python`, `curl`, `Go-http-client` and `httpx`.

resource "cloudflare_ruleset" "cache" {
  zone_id = data.cloudflare_zone.freehire.id
  name    = "freehire cache rules"
  kind    = "zone"
  phase   = "http_request_cache_settings"

  # Never cache the API, the account area, or anything that isn't a read. The
  # origin marks these `private, no-store` already; this is the belt to that
  # braces, so a caching mistake needs two independent failures.
  rules {
    description = "Bypass: API, account routes, non-GET"
    expression  = <<-EOT
      (starts_with(http.request.uri.path, "/api/")
       or starts_with(http.request.uri.path, "/my/")
       or starts_with(http.request.uri.path, "/auth/")
       or starts_with(http.request.uri.path, "/moderation")
       or starts_with(http.request.uri.path, "/cv/")
       or http.request.method ne "GET")
    EOT
    action      = "set_cache_settings"
    enabled     = true

    action_parameters {
      cache = false
    }
  }

  # Everything else: cacheable, on the origin's terms. A signed-in response says
  # `private, no-store` and is therefore not stored, which is what keeps one
  # visitor's page from being replayed to another.
  rules {
    description = "Cache HTML per origin headers"
    expression  = "true"
    action      = "set_cache_settings"
    enabled     = true

    action_parameters {
      cache = true

      edge_ttl {
        mode = "respect_origin"
      }

      browser_ttl {
        mode = "respect_origin"
      }

      # The session cookie changes the response, so it must change the cache key.
      cache_key {
        cache_by_device_type = false

        custom_key {
          cookie {
            check_presence = ["hire_token"]
          }
        }
      }
    }
  }
}
