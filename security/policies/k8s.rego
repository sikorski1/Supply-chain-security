package supplychain.policy

import rego.v1

deny contains msg if {
	object.get(input.deployment, "kind", "") != "Deployment"
	msg := "Rendered Kubernetes manifest must be a Deployment."
}

deny contains msg if {
	actual := object.get(object.get(input.deployment, "metadata", {}), "name", "")
	actual != input.context.component
	msg := sprintf("Deployment name %q does not match component %q.", [actual, input.context.component])
}

pod_spec := object.get(object.get(object.get(input.deployment, "spec", {}), "template", {}), "spec", {})

deny contains msg if {
	count(object.get(pod_spec, "containers", [])) == 0
	msg := "Deployment pod template must define at least one container."
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	not startswith(c.image, input.context.allowed_registry_prefix)
	msg := sprintf("Container %q image %q must start with %q.", [c.name, c.image, input.context.allowed_registry_prefix])
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	not contains(c.image, "@sha256:")
	msg := sprintf("Container %q image %q must be pinned by @sha256 digest.", [c.name, c.image])
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	not endswith(c.image, sprintf("@%s", [input.context.image_digest]))
	msg := sprintf("Container %q image %q does not match verified digest %s.", [c.name, c.image, input.context.image_digest])
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	object.get(c, "imagePullPolicy", "") != "Always"
	msg := sprintf("Container %q must set imagePullPolicy: Always.", [c.name])
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	not c.readinessProbe
	msg := sprintf("Container %q must define a readinessProbe.", [c.name])
}

deny contains msg if {
	some c in object.get(pod_spec, "containers", [])
	object.get(object.get(c, "securityContext", {}), "privileged", false) == true
	msg := sprintf("Container %q must not run privileged.", [c.name])
}

deny contains msg if {
	object.get(pod_spec, "hostNetwork", false) == true
	msg := "Deployment must not enable hostNetwork."
}

deny contains msg if {
	object.get(pod_spec, "hostPID", false) == true
	msg := "Deployment must not enable hostPID."
}

deny contains msg if {
	object.get(pod_spec, "hostIPC", false) == true
	msg := "Deployment must not enable hostIPC."
}
