package store

import "strings"

// Prefixes for the "<TYPE>#<ID>" key scheme (docs/engineering/dynamodb.md
// §1). No caller outside this file should concatenate these by hand — every
// key goes through the helpers below.
const (
	devicePrefix  = "DEVICE#"
	pairingPrefix = "PAIRING#"
	obsPrefix     = "OBS#"
	ownerPrefix   = "OWNER#"

	// archivedGSI1PK is the single, deliberately hot partition that
	// ARCHIVED devices are swapped into (§4). No reader in this package
	// queries it — docs/01-openquestion.md DATA-02 puts that cleanup scan
	// out of MVP scope — but Task 7's ArchiveDevice needs the constant.
	archivedGSI1PK = "ARCHIVED"
)

// deviceKey returns "DEVICE#<deviceId>", used as both PK and SK for a
// Device item, and as the PK for every item under that device's partition
// (PairingSession, Observation).
func deviceKey(deviceID string) string {
	return devicePrefix + deviceID
}

// pairingSK returns the base-table SK for a PairingSession item.
func pairingSK(pairingCode string) string {
	return pairingPrefix + pairingCode
}

// observationSK returns the base-table SK for an Observation item.
func observationSK(observationID string) string {
	return obsPrefix + observationID
}

// ownerGSI1PK returns GSI1's partition key for an owner's Device and
// PairingSession rows.
func ownerGSI1PK(ownerID string) string {
	return ownerPrefix + ownerID
}

// deviceGSI1SK returns GSI1's sort key for a (non-archived) Device row.
func deviceGSI1SK(deviceID string) string {
	return devicePrefix + deviceID
}

// pairingGSI1SK returns GSI1's sort key for a PairingSession row. Note this
// is keyed by deviceId, not pairingCode: GSI1's job is "list an owner's
// devices, with pairing status", not "find a session by its code" (that is
// GSI2's job).
func pairingGSI1SK(deviceID string) string {
	return pairingPrefix + deviceID
}

// pairingGSI2Key returns GSI2's partition (and sort) key for a
// PairingSession row. Both are the same value: GSI2's only job is mapping a
// scanned pairingCode back to the base table's PK/SK, so the "range" part
// carries no extra information.
func pairingGSI2Key(pairingCode string) string {
	return pairingPrefix + pairingCode
}

// ownerIDFromGSI1PK recovers the ownerId from a GSI1PK value of the form
// "OWNER#<ownerId>". GSI1's INCLUDE projection does not carry ownerId as an
// attribute (docs/engineering/dynamodb.md §4), so this is the only way to
// recover it from a GSI1 row — see OwnerListRow.OwnerID.
func ownerIDFromGSI1PK(gsi1pk string) string {
	return strings.TrimPrefix(gsi1pk, ownerPrefix)
}
