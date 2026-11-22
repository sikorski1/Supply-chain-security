package supplychain.policy

import rego.v1

default has_valid_signature := false

has_valid_signature if {
	some entry in object.get(input, "cosign_verify", [])
	entry.critical.image["docker-manifest-digest"] == input.context.image_digest
}

deny contains msg if {
	not has_valid_signature
	msg := sprintf("No verified Cosign signature matching digest %s.", [input.context.image_digest])
}

expected_sha256_hex := trim_prefix(input.context.image_digest, "sha256:")

statement_matches_subject(stmt) if {
	some subj in object.get(stmt, "subject", [])
	subj.digest.sha256 == expected_sha256_hex
}

statement_matches_commit(stmt) if {
	some dep in object.get(object.get(stmt.predicate, "buildDefinition", {}), "resolvedDependencies", [])
	dep.digest.gitCommit == input.context.commit_sha
}

default has_valid_provenance := false

has_valid_provenance if {
	some stmt in object.get(input, "provenance_statements", [])
	stmt.predicateType == "https://slsa.dev/provenance/v1"
	statement_matches_subject(stmt)
	bd := stmt.predicate.buildDefinition
	bd.buildType == "https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1"
	bd.externalParameters.workflow.repository == input.context.trusted_repository
	bd.externalParameters.component == input.context.component
	statement_matches_commit(stmt)
}

deny contains msg if {
	not has_valid_provenance
	msg := sprintf(
		"No verified SLSA Provenance v1.0 attestation matching digest %s, repo %s, component %s, commit %s.",
		[input.context.image_digest, input.context.trusted_repository, input.context.component, input.context.commit_sha],
	)
}
