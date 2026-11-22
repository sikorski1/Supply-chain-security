package supplychain.policy

import rego.v1

deny contains msg if {
	not input.sbom_source
	msg := "Missing source SBOM artifact."
}

deny contains msg if {
	input.sbom_source
	object.get(input.sbom_source, "bomFormat", "") != "CycloneDX"
	msg := sprintf("Source SBOM bomFormat must be CycloneDX, got %q.", [object.get(input.sbom_source, "bomFormat", "")])
}

deny contains msg if {
	input.sbom_source
	count(object.get(input.sbom_source, "components", [])) == 0
	msg := "Source SBOM must contain at least one component."
}

deny contains msg if {
	input.sbom_source
	actual := object.get(object.get(object.get(input.sbom_source, "metadata", {}), "component", {}), "name", "")
	actual != input.context.component
	msg := sprintf("Source SBOM component name %q does not match expected %q.", [actual, input.context.component])
}

deny contains msg if {
	input.sbom_source
	actual := object.get(object.get(object.get(input.sbom_source, "metadata", {}), "component", {}), "version", "")
	actual != input.context.commit_sha
	msg := sprintf("Source SBOM commit version %q does not match expected %q.", [actual, input.context.commit_sha])
}

deny contains msg if {
	not input.sbom_image
	msg := "Missing container image SBOM artifact."
}

deny contains msg if {
	input.sbom_image
	object.get(input.sbom_image, "bomFormat", "") != "CycloneDX"
	msg := sprintf("Image SBOM bomFormat must be CycloneDX, got %q.", [object.get(input.sbom_image, "bomFormat", "")])
}

deny contains msg if {
	input.sbom_image
	count(object.get(input.sbom_image, "components", [])) == 0
	msg := "Image SBOM must contain at least one component."
}
