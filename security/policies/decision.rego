package supplychain.policy

import rego.v1

default allow := false

allow if {
	count(deny) == 0
}

result := {
	"allow": allow,
	"deny_count": count(deny),
	"deny": sort(deny),
	"warn_count": count(warn),
	"warn": sort(warn),
	"summary": {
		"sbom_source_components": count(object.get(input.sbom_source, "components", [])),
		"sbom_image_components": count(object.get(input.sbom_image, "components", [])),
		"vulnerabilities": vulnerability_summary,
		"signature_verified": has_valid_signature,
		"provenance_verified": has_valid_provenance,
	},
}
