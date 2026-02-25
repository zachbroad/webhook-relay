variable "tenancy_ocid" {
  description = "OCID of the OCI tenancy"
  type        = string
}

variable "user_ocid" {
  description = "OCID of the OCI user"
  type        = string
}

variable "fingerprint" {
  description = "Fingerprint of the OCI API signing key"
  type        = string
}

variable "private_key_path" {
  description = "Path to the OCI API private key"
  type        = string
}

variable "region" {
  description = "OCI region"
  type        = string
}

variable "compartment_ocid" {
  description = "OCID of the compartment to deploy resources into"
  type        = string
}

variable "kubernetes_version" {
  description = "Kubernetes version for the OKE cluster and node pool"
  type        = string
  default     = "v1.34.2"
}

variable "node_shape" {
  description = "Shape for the worker nodes"
  type        = string
  default     = "VM.Standard.A1.Flex"
}

variable "node_ocpus" {
  description = "Number of OCPUs per worker node"
  type        = number
  default     = 2
}

variable "node_memory_in_gbs" {
  description = "Memory in GBs per worker node"
  type        = number
  default     = 12
}

variable "node_pool_size" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}

variable "node_boot_volume_size_in_gbs" {
  description = "Boot volume size in GBs per worker node"
  type        = number
  default     = 100
}

variable "ssh_public_key_path" {
  description = "Path to SSH public key for worker nodes"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}
