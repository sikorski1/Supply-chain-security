package supplychain.policy

import rego.v1

deny contains msg if {
	not input.trivy_source
	msg := "Missing Trivy source vulnerability report."
}

deny contains msg if {
	input.trivy_source
	object.get(input.trivy_source, "SchemaVersion", 0) != 2
	msg := sprintf("Trivy source report SchemaVersion must be 2, got %v.", [object.get(input.trivy_source, "SchemaVersion", 0)])
}

deny contains msg if {
	not input.trivy_image
	msg := "Missing Trivy image vulnerability report."
}

deny contains msg if {
	input.trivy_image
	object.get(input.trivy_image, "SchemaVersion", 0) != 2
	msg := sprintf("Trivy image report SchemaVersion must be 2, got %v.", [object.get(input.trivy_image, "SchemaVersion", 0)])
}

# Hard gate: zero CRITICAL vulnerabilities allowed in application source dependencies.
deny contains msg if {
	some res in object.get(input.trivy_source, "Results", [])
	some vuln in object.get(res, "Vulnerabilities", [])
	vuln.Severity == "CRITICAL"
	msg := sprintf("Critical source vulnerability %s in %s (%s).", [vuln.VulnerabilityID, vuln.PkgName, res.Target])
}

# Audit warning: traverse and report CRITICAL/HIGH base-image vulnerabilities without blocking pinned benchmark images.
warn contains msg if {
	some res in object.get(input.trivy_image, "Results", [])
	some vuln in object.get(res, "Vulnerabilities", [])
	vuln.Severity in {"CRITICAL", "HIGH"}
	msg := sprintf("Image vulnerability [%s] %s in %s (%s).", [vuln.Severity, vuln.VulnerabilityID, vuln.PkgName, res.Target])
}

source_vulns := [v |
	some res in object.get(input.trivy_source, "Results", [])
	some v in object.get(res, "Vulnerabilities", [])
]

image_vulns := [v |
	some res in object.get(input.trivy_image, "Results", [])
	some v in object.get(res, "Vulnerabilities", [])
]

count_severity(vulns, sev) := count([v | some v in vulns; v.Severity == sev])

vulnerability_summary := {
	"source": {
		"total": count(source_vulns),
		"CRITICAL": count_severity(source_vulns, "CRITICAL"),
		"HIGH": count_severity(source_vulns, "HIGH"),
		"MEDIUM": count_severity(source_vulns, "MEDIUM"),
		"LOW": count_severity(source_vulns, "LOW"),
		"UNKNOWN": count_severity(source_vulns, "UNKNOWN"),
	},
	"image": {
		"total": count(image_vulns),
		"CRITICAL": count_severity(image_vulns, "CRITICAL"),
		"HIGH": count_severity(image_vulns, "HIGH"),
		"MEDIUM": count_severity(image_vulns, "MEDIUM"),
		"LOW": count_severity(image_vulns, "LOW"),
		"UNKNOWN": count_severity(image_vulns, "UNKNOWN"),
	},
}
