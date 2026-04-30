# Configure the IPAM provider with endpoint.
# Token can be set here or via IPAM_TOKEN environment variable.
provider "ipam" {
  endpoint = "https://ipam.example.com"
  # token = var.ipam_token
}

variable "ipam_token" {
  type      = string
  sensitive = true
  default   = ""
}
