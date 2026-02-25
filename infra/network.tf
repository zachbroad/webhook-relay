resource "oci_core_vcn" "cluster" {
  compartment_id = var.compartment_ocid
  cidr_blocks    = ["10.0.0.0/16"]
  display_name   = "oke-vcn-quick-cluster1-a11b69ec2"
  dns_label      = "cluster1"
}

resource "oci_core_internet_gateway" "cluster" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-igw-quick-cluster1-a11b69ec2"
  enabled        = true
  vcn_id         = oci_core_vcn.cluster.id
}

resource "oci_core_nat_gateway" "cluster" {
  compartment_id = var.compartment_ocid
  block_traffic  = false
  display_name   = "oke-ngw-quick-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
}

resource "oci_core_service_gateway" "cluster" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-sgw-quick-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
  services {
    service_id = data.oci_core_services.all.services[0].id
  }
}

resource "oci_core_dhcp_options" "cluster" {
  compartment_id   = var.compartment_ocid
  display_name     = "Default DHCP Options for oke-vcn-quick-cluster1-a11b69ec2"
  domain_name_type = "CUSTOM_DOMAIN"
  vcn_id           = oci_core_vcn.cluster.id
  options {
    search_domain_names = ["cluster1.oraclevcn.com"]
    type                = "SearchDomain"
  }
  options {
    server_type = "VcnLocalPlusInternet"
    type        = "DomainNameServer"
  }
}

# Route Tables

resource "oci_core_route_table" "public" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-public-routetable-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
  route_rules {
    description       = "traffic to/from internet"
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.cluster.id
  }
}

resource "oci_core_route_table" "private" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-private-routetable-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
  route_rules {
    description       = "traffic to OCI services"
    destination       = "all-ord-services-in-oracle-services-network"
    destination_type  = "SERVICE_CIDR_BLOCK"
    network_entity_id = oci_core_service_gateway.cluster.id
  }
  route_rules {
    description       = "traffic to the internet"
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_nat_gateway.cluster.id
  }
}

# Security Lists

resource "oci_core_security_list" "svclb" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-svclbseclist-quick-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
}

resource "oci_core_security_list" "k8s_api" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-k8sApiEndpoint-quick-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
  egress_security_rules {
    description      = "All traffic to worker nodes"
    destination      = "10.0.10.0/24"
    destination_type = "CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
  }
  egress_security_rules {
    description      = "Allow Kubernetes Control Plane to communicate with OKE"
    destination      = "all-ord-services-in-oracle-services-network"
    destination_type = "SERVICE_CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
    tcp_options {
      max = 443
      min = 443
    }
  }
  egress_security_rules {
    description      = "Path discovery"
    destination      = "10.0.10.0/24"
    destination_type = "CIDR_BLOCK"
    protocol         = "1"
    stateless        = false
    icmp_options {
      code = 4
      type = 3
    }
  }
  ingress_security_rules {
    description = "External access to Kubernetes API endpoint"
    protocol    = "6"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 6443
      min = 6443
    }
  }
  ingress_security_rules {
    description = "Kubernetes worker to Kubernetes API endpoint communication"
    protocol    = "6"
    source      = "10.0.10.0/24"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 6443
      min = 6443
    }
  }
  ingress_security_rules {
    description = "Kubernetes worker to control plane communication"
    protocol    = "6"
    source      = "10.0.10.0/24"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 12250
      min = 12250
    }
  }
  ingress_security_rules {
    description = "Path discovery"
    protocol    = "1"
    source      = "10.0.10.0/24"
    source_type = "CIDR_BLOCK"
    stateless   = false
    icmp_options {
      code = 4
      type = 3
    }
  }
}

resource "oci_core_security_list" "node" {
  compartment_id = var.compartment_ocid
  display_name   = "oke-nodeseclist-quick-cluster1-a11b69ec2"
  vcn_id         = oci_core_vcn.cluster.id
  egress_security_rules {
    description      = "Access to Kubernetes API Endpoint"
    destination      = "10.0.0.0/28"
    destination_type = "CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
    tcp_options {
      max = 6443
      min = 6443
    }
  }
  egress_security_rules {
    description      = "Allow nodes to communicate with OKE to ensure correct start-up and continued functioning"
    destination      = "all-ord-services-in-oracle-services-network"
    destination_type = "SERVICE_CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
    tcp_options {
      max = 443
      min = 443
    }
  }
  egress_security_rules {
    description      = "Allow pods on one worker node to communicate with pods on other worker nodes"
    destination      = "10.0.10.0/24"
    destination_type = "CIDR_BLOCK"
    protocol         = "all"
    stateless        = false
  }
  egress_security_rules {
    description      = "ICMP Access from Kubernetes Control Plane"
    destination      = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    protocol         = "1"
    stateless        = false
    icmp_options {
      code = 4
      type = 3
    }
  }
  egress_security_rules {
    description      = "Kubernetes worker to control plane communication"
    destination      = "10.0.0.0/28"
    destination_type = "CIDR_BLOCK"
    protocol         = "6"
    stateless        = false
    tcp_options {
      max = 12250
      min = 12250
    }
  }
  egress_security_rules {
    description      = "Path discovery"
    destination      = "10.0.0.0/28"
    destination_type = "CIDR_BLOCK"
    protocol         = "1"
    stateless        = false
    icmp_options {
      code = 4
      type = 3
    }
  }
  egress_security_rules {
    description      = "Worker Nodes access to Internet"
    destination      = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    protocol         = "all"
    stateless        = false
  }
  ingress_security_rules {
    description = "Allow pods on one worker node to communicate with pods on other worker nodes"
    protocol    = "all"
    source      = "10.0.10.0/24"
    source_type = "CIDR_BLOCK"
    stateless   = false
  }
  ingress_security_rules {
    description = "Inbound SSH traffic to worker nodes"
    protocol    = "6"
    source      = "0.0.0.0/0"
    source_type = "CIDR_BLOCK"
    stateless   = false
    tcp_options {
      max = 22
      min = 22
    }
  }
  ingress_security_rules {
    description = "Path discovery"
    protocol    = "1"
    source      = "10.0.0.0/28"
    source_type = "CIDR_BLOCK"
    stateless   = false
    icmp_options {
      code = 4
      type = 3
    }
  }
  ingress_security_rules {
    description = "TCP access from Kubernetes Control Plane"
    protocol    = "6"
    source      = "10.0.0.0/28"
    source_type = "CIDR_BLOCK"
    stateless   = false
  }
}

# Subnets

resource "oci_core_subnet" "k8s_api" {
  compartment_id             = var.compartment_ocid
  cidr_block                 = "10.0.0.0/28"
  display_name               = "oke-k8sApiEndpoint-subnet-quick-cluster1-a11b69ec2-regional"
  dns_label                  = "sub59ef41df3"
  dhcp_options_id            = oci_core_dhcp_options.cluster.id
  prohibit_internet_ingress  = false
  prohibit_public_ip_on_vnic = false
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.k8s_api.id]
  vcn_id                     = oci_core_vcn.cluster.id
}

resource "oci_core_subnet" "node" {
  compartment_id             = var.compartment_ocid
  cidr_block                 = "10.0.10.0/24"
  display_name               = "oke-nodesubnet-quick-cluster1-a11b69ec2-regional"
  dns_label                  = "sub055072f61"
  dhcp_options_id            = oci_core_dhcp_options.cluster.id
  prohibit_internet_ingress  = true
  prohibit_public_ip_on_vnic = true
  route_table_id             = oci_core_route_table.private.id
  security_list_ids          = [oci_core_security_list.node.id]
  vcn_id                     = oci_core_vcn.cluster.id
}

resource "oci_core_subnet" "svclb" {
  compartment_id             = var.compartment_ocid
  cidr_block                 = "10.0.20.0/24"
  display_name               = "oke-svclbsubnet-quick-cluster1-a11b69ec2-regional"
  dns_label                  = "lbsube57d0d681"
  dhcp_options_id            = oci_core_dhcp_options.cluster.id
  prohibit_internet_ingress  = false
  prohibit_public_ip_on_vnic = false
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.svclb.id]
  vcn_id                     = oci_core_vcn.cluster.id
}
