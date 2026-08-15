variable "cloudflare_api_token" {
  description = "Cloudflare API token: Zone Settings:Edit, DNS:Edit, Cache Rules:Edit on this zone"
  type        = string
  sensitive   = true
}

variable "zone_name" {
  description = "Domain name"
  type        = string
  default     = "freehire.me"
}

variable "origin_server_ip" {
  description = "Hetzner origin the web container is published on"
  type        = string
  default     = "89.167.94.146"
}
