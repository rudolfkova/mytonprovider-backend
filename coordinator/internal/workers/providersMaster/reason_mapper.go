package providersmaster

import (
	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-coordinator/internal/constants"
)

func reasonFromProto(code providerchecksv1.ReasonCode) constants.ReasonCode {
	switch code {
	case providerchecksv1.ReasonCode_VALID_STORAGE_PROOF:
		return constants.ValidStorageProof
	case providerchecksv1.ReasonCode_IP_NOT_FOUND:
		return constants.IPNotFound
	case providerchecksv1.ReasonCode_NOT_FOUND:
		return constants.NotFound
	case providerchecksv1.ReasonCode_UNAVAILABLE_PROVIDER:
		return constants.UnavailableProvider
	case providerchecksv1.ReasonCode_CANT_CREATE_PEER:
		return constants.CantCreatePeer
	case providerchecksv1.ReasonCode_UNKNOWN_PEER:
		return constants.UnknownPeer
	case providerchecksv1.ReasonCode_PING_FAILED:
		return constants.PingFailed
	case providerchecksv1.ReasonCode_INVALID_BAG_ID:
		return constants.InvalidBagID
	case providerchecksv1.ReasonCode_FAILED_INITIAL_PING:
		return constants.FailedInitialPing
	case providerchecksv1.ReasonCode_GET_INFO_FAILED:
		return constants.GetInfoFailed
	case providerchecksv1.ReasonCode_INVALID_HEADER:
		return constants.InvalidHeader
	case providerchecksv1.ReasonCode_CANT_GET_PIECE:
		return constants.CantGetPiece
	case providerchecksv1.ReasonCode_CANT_PARSE_BOC:
		return constants.CantParseBoC
	case providerchecksv1.ReasonCode_PROOF_CHECK_FAILED:
		return constants.ProofCheckFailed
	default:
		return constants.NotFound
	}
}
