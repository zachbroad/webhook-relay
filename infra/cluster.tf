resource "oci_containerengine_cluster" "cluster" {
  compartment_id     = var.compartment_ocid
  kubernetes_version = var.kubernetes_version
  name               = "cluster1"
  type               = "BASIC_CLUSTER"
  vcn_id             = oci_core_vcn.cluster.id
  cluster_pod_network_options {
    cni_type = "OCI_VCN_IP_NATIVE"
  }
  endpoint_config {
    is_public_ip_enabled = true
    subnet_id            = oci_core_subnet.k8s_api.id
  }
  options {
    ip_families           = ["IPv4"]
    service_lb_subnet_ids = [oci_core_subnet.svclb.id]
    kubernetes_network_config {
      pods_cidr     = "10.244.0.0/16"
      services_cidr = "10.96.0.0/16"
    }
  }
}

resource "oci_containerengine_node_pool" "pool1" {
  cluster_id         = oci_containerengine_cluster.cluster.id
  compartment_id     = var.compartment_ocid
  kubernetes_version = var.kubernetes_version
  name               = "pool1"
  node_shape         = var.node_shape
  ssh_public_key     = file(var.ssh_public_key_path)
  initial_node_labels {
    key   = "name"
    value = "cluster1"
  }
  node_config_details {
    size = var.node_pool_size
    node_pool_pod_network_option_details {
      cni_type          = "OCI_VCN_IP_NATIVE"
      max_pods_per_node = 31
      pod_subnet_ids    = [oci_core_subnet.node.id]
    }
    dynamic "placement_configs" {
      for_each = data.oci_identity_availability_domains.ads.availability_domains
      content {
        availability_domain = placement_configs.value.name
        subnet_id           = oci_core_subnet.node.id
      }
    }
  }
  node_eviction_node_pool_settings {
    eviction_grace_duration = "PT1H"
  }
  node_shape_config {
    memory_in_gbs = var.node_memory_in_gbs
    ocpus         = var.node_ocpus
  }
  node_source_details {
    boot_volume_size_in_gbs = var.node_boot_volume_size_in_gbs
    image_id                = [for s in data.oci_containerengine_node_pool_option.current.sources : s.image_id if length(regexall("Oracle-Linux.*aarch64.*OKE-${replace(var.kubernetes_version, "v", "")}", s.source_name)) > 0][0]
    source_type             = "IMAGE"
  }
}
