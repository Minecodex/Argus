package toolruntime

// NativeQueryEvidenceFacts supplies the fixed evidence contract at admission
// and inference. Callers alone establish current authorization and availability;
// an empty prospective Run has no originals and is complete.
func NativeQueryEvidenceFacts(items any, complete bool) map[string]any {
	return map[string]any{
		"items": items, "complete": complete,
		"semantics": "Current authorized Run query originals, at most64. inline_original_in_context=true means the complete small original is present in an actual tool message of THIS request and may be analyzed directly. inline_exposure_verified=true independently proves that this exact original was fully included in an earlier successful actual provider request; established facts may be reused after compaction, but do not claim its full original is still in the current context. For fresh analysis requiring absent details, read the contributing original. If neither current complete inline nor verified prior full exposure nor complete_read_verified applies, read/analyze the original file; large/partial/truncated originals require file analysis. not_verified_by_framework includes arbitrary bash analysis and is not proof of failure. Do not claim physical file reads from inline analysis. These facts grant no access, numeric correctness, sampling coverage or business semantics. Answer briefly in the requested language; use existing evidence cards instead of reproducing them or exposing internal framework field names. Omit unsolicited calculations or tutorials.",
	}
}
