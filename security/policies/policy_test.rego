package supplychain.policy_test

import rego.v1

import data.supplychain.policy

valid_digest := "sha256:7ab575d8762efc599ab920e6d4aaffce3f5e05184f1041bd2e146dc17b29fdf9"
valid_hex := "7ab575d8762efc599ab920e6d4aaffce3f5e05184f1041bd2e146dc17b29fdf9"
valid_commit := "390d40a000000000000000000000000000000000"
valid_repo := "https://github.com/sikorski1/Supply-chain-security"

valid_input := {
	"context": {
		"component": "inventory",
		"commit_sha": valid_commit,
		"image_digest": valid_digest,
		"trusted_repository": valid_repo,
		"allowed_registry_prefix": "localhost:5001/",
	},
	"sbom_source": {
		"bomFormat": "CycloneDX",
		"metadata": {"component": {"name": "inventory", "version": valid_commit}},
		"components": [{"name": "chi", "version": "5.5.0"}],
	},
	"sbom_image": {
		"bomFormat": "CycloneDX",
		"components": [{"name": "alpine-baselayout", "version": "3.7.0"}],
	},
	"trivy_source": {
		"SchemaVersion": 2,
		"Results": [{"Target": "go.mod", "Vulnerabilities": [{"VulnerabilityID": "CVE-2026-0001", "PkgName": "foo", "Severity": "MEDIUM"}]}],
	},
	"trivy_image": {
		"SchemaVersion": 2,
		"Results": [{"Target": "alpine", "Vulnerabilities": [{"VulnerabilityID": "CVE-2026-0002", "PkgName": "openssl", "Severity": "HIGH"}]}],
	},
	"cosign_verify": [
		{"critical": {"image": {"docker-manifest-digest": valid_digest}}},
	],
	"provenance_statements": [
		{
			"_type": "https://in-toto.io/Statement/v0.1",
			"predicateType": "https://slsa.dev/provenance/v1",
			"subject": [{"name": "localhost:5001/inventory", "digest": {"sha256": valid_hex}}],
			"predicate": {
				"buildDefinition": {
					"buildType": "https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1",
					"externalParameters": {
						"workflow": {"repository": valid_repo, "ref": "refs/heads/main", "path": ".github/workflows/benchmark-l5.yaml"},
						"component": "inventory",
					},
					"resolvedDependencies": [{"uri": "git+repo", "digest": {"gitCommit": valid_commit}}],
				},
			},
		},
	],
	"deployment": {
		"kind": "Deployment",
		"metadata": {"name": "inventory"},
		"spec": {
			"template": {
				"spec": {
					"containers": [
						{
							"name": "inventory",
							"image": sprintf("localhost:5001/inventory:l5-1-1@%s", [valid_digest]),
							"imagePullPolicy": "Always",
							"readinessProbe": {"httpGet": {"path": "/health", "port": 8082}},
						},
					],
				},
			},
		},
	},
}

test_valid_input_allowed_with_image_warning if {
	res := policy.result with input as valid_input
	res.allow == true
	res.deny_count == 0
	res.warn_count == 1
}

test_critical_source_cve_denied if {
	bad := object.union(valid_input, {
		"trivy_source": {
			"SchemaVersion": 2,
			"Results": [{"Target": "go.mod", "Vulnerabilities": [{"VulnerabilityID": "CVE-2026-9999", "PkgName": "badpkg", "Severity": "CRITICAL"}]}],
		},
	})
	res := policy.result with input as bad
	res.allow == false
	res.deny_count == 1
}

test_missing_signature_denied if {
	bad := object.union(valid_input, {"cosign_verify": []})
	res := policy.result with input as bad
	res.allow == false
}

test_untrusted_provenance_repo_denied if {
	bad_stmt := json.patch(valid_input.provenance_statements, [{
		"op": "replace",
		"path": "/0/predicate/buildDefinition/externalParameters/workflow/repository",
		"value": "https://github.com/attacker/evil",
	}])
	bad := object.union(valid_input, {"provenance_statements": bad_stmt})
	res := policy.result with input as bad
	res.allow == false
}

test_mutable_tag_deployment_denied if {
	bad_dep := json.patch(valid_input.deployment, [{
		"op": "replace",
		"path": "/spec/template/spec/containers/0/image",
		"value": "localhost:5001/inventory:l5-1-1",
	}])
	bad := object.union(valid_input, {"deployment": bad_dep})
	res := policy.result with input as bad
	res.allow == false
}

test_privileged_container_denied if {
	bad_dep := json.patch(valid_input.deployment, [{
		"op": "add",
		"path": "/spec/template/spec/containers/0/securityContext",
		"value": {"privileged": true},
	}])
	bad := object.union(valid_input, {"deployment": bad_dep})
	res := policy.result with input as bad
	res.allow == false
}
