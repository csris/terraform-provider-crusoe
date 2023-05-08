package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/antihax/optional"

	swagger "gitlab.com/crusoeenergy/island/external/client-go/swagger/v1alpha4"
)

type opStatus string

type opResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	// TODO: is there a way we can avoid redefining these here?
	OpSucceeded  opStatus = "SUCCEEDED"
	OpInProgress opStatus = "IN_PROGRESS"
	OpFailed     opStatus = "FAILED"

	errNoOperations     = errors.New("no operation with id found")
	errUnableToGetOpRes = errors.New("failed to get result of operation")

	errAmbiguousRole     = errors.New("user is associated with multiple roles - please contact support")
	errNoRoleAssociation = errors.New("user is not associated with any role")

	// fallback error presented to the user in unexpected situations
	errUnexpected = errors.New("An unexpected error occurred. Please try again, and if the problem persists, contact support.")

	pollInterval = 2 * time.Second
)

// NewAPIClient initializes a new Crusoe API client with the given configuration.
func NewAPIClient(host, key, secret string) *swagger.APIClient {
	cfg := swagger.NewConfiguration()
	cfg.UserAgent = "CrusoeTerraform/0.0.1" // TODO: include version
	cfg.BasePath = host
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	cfg.HTTPClient.Transport = NewAuthenticatingTransport(cfg.HTTPClient.Transport, key, secret)

	return swagger.NewAPIClient(cfg)
}

// GetRole creates a get Role request and calls the API.
// This function returns a role id if the user's role can be determined
// (i.e. user only has one role, which is the case for v0).
func GetRole(ctx context.Context, api *swagger.APIClient) (string, error) {
	opts := swagger.RolesApiGetRolesOpts{
		OrgId: optional.EmptyString(),
	}

	resp, httpResp, err := api.RolesApi.GetRoles(ctx, &opts)
	if err != nil {
		return "", fmt.Errorf("could not get roles: %w", err)
	}
	defer httpResp.Body.Close()

	switch len(resp.Roles) {
	case 0:
		return "", errNoRoleAssociation
	case 1:
		return resp.Roles[0].Id, nil
	default:
		// user has multiple roles: unable to disambiguate
		return "", errAmbiguousRole
	}
}

// AwaitOperation polls an async API operation until it resolves into a success or failure state.
func AwaitOperation(ctx context.Context, op *swagger.Operation,
	getFunc func(context.Context, string) (swagger.OperationsGetResponse, *http.Response, error)) (
	*swagger.Operation, error,
) {
	for op.State == string(OpInProgress) {
		updatedOps, httpResp, err := getFunc(ctx, op.OperationId)
		if err != nil {
			return nil, fmt.Errorf("error getting operation with id %s: %w", op.OperationId, err)
		}
		httpResp.Body.Close()
		if len(updatedOps.Operations) == 0 {
			return nil, errNoOperations
		}
		op = &updatedOps.Operations[0]

		time.Sleep(pollInterval)
	}

	switch op.State {
	case string(OpSucceeded):
		return op, nil
	case string(OpFailed):
		opError, err := opResultToError(op.Result)
		if err != nil {
			return op, err
		}
		return op, opError
	default:
		return op, errUnexpected
	}
}

// AwaitOperationAndResolve awaits an async API operation and attempts to parse the response as an instance of T,
// assuming the operation was successful.
func AwaitOperationAndResolve[T any](ctx context.Context, op *swagger.Operation,
	getFunc func(context.Context, string) (swagger.OperationsGetResponse, *http.Response, error)) (*T, *swagger.Operation, error) {
	op, err := AwaitOperation(ctx, op, getFunc)
	if err != nil {
		return nil, op, err
	}

	result, err := parseOpResult[T](op.Result)
	if err != nil {
		return nil, op, err
	}

	return result, op, nil
}

func parseOpResult[T any](opResult interface{}) (*T, error) {
	b, err := json.Marshal(opResult)
	if err != nil {
		return nil, errUnableToGetOpRes
	}

	var result T
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, errUnableToGetOpRes
	}

	return &result, nil
}

func opResultToError(res interface{}) (expectedErr, unexpectedErr error) {
	b, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal operation error: %w", err)
	}
	resultError := opResultError{}
	err = json.Unmarshal(b, &resultError)
	if err != nil {
		return nil, fmt.Errorf("op result type not error as expected: %w", err)
	}

	//nolint:goerr113 //This function is designed to return dynamic errors
	return fmt.Errorf("%s", resultError.Message), nil
}