package reason

import providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"

type Code uint32

const (
	ValidStorageProof Code = 0

	IPNotFound          Code = 101
	NotFound            Code = 102
	UnavailableProvider Code = 103
	CantCreatePeer      Code = 104
	UnknownPeer         Code = 105

	PingFailed        Code = 201
	InvalidBagID      Code = 202
	FailedInitialPing Code = 203

	GetInfoFailed Code = 301
	InvalidHeader Code = 302

	CantGetPiece     Code = 401
	CantParseBoC     Code = 402
	ProofCheckFailed Code = 403
)

func ToProto(code Code) providerchecksv1.ReasonCode {
	switch code {
	case ValidStorageProof:
		return providerchecksv1.ReasonCode_VALID_STORAGE_PROOF
	case IPNotFound:
		return providerchecksv1.ReasonCode_IP_NOT_FOUND
	case NotFound:
		return providerchecksv1.ReasonCode_NOT_FOUND
	case UnavailableProvider:
		return providerchecksv1.ReasonCode_UNAVAILABLE_PROVIDER
	case CantCreatePeer:
		return providerchecksv1.ReasonCode_CANT_CREATE_PEER
	case UnknownPeer:
		return providerchecksv1.ReasonCode_UNKNOWN_PEER
	case PingFailed:
		return providerchecksv1.ReasonCode_PING_FAILED
	case InvalidBagID:
		return providerchecksv1.ReasonCode_INVALID_BAG_ID
	case FailedInitialPing:
		return providerchecksv1.ReasonCode_FAILED_INITIAL_PING
	case GetInfoFailed:
		return providerchecksv1.ReasonCode_GET_INFO_FAILED
	case InvalidHeader:
		return providerchecksv1.ReasonCode_INVALID_HEADER
	case CantGetPiece:
		return providerchecksv1.ReasonCode_CANT_GET_PIECE
	case CantParseBoC:
		return providerchecksv1.ReasonCode_CANT_PARSE_BOC
	case ProofCheckFailed:
		return providerchecksv1.ReasonCode_PROOF_CHECK_FAILED
	default:
		return providerchecksv1.ReasonCode_REASON_CODE_UNSPECIFIED
	}
}
