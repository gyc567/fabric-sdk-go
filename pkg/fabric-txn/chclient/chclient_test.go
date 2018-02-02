/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chclient

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric-sdk-go/api/apifabclient"
	"github.com/hyperledger/fabric-sdk-go/api/apitxn/chclient"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric-sdk-go/pkg/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/errors/status"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/channel"
	fcmocks "github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/mocks"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-client/peer"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-txn/internal"
	txnmocks "github.com/hyperledger/fabric-sdk-go/pkg/fabric-txn/mocks"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabric-txn/txnhandler"
	pb "github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

const (
	testAddress = "127.0.0.1:47882"
)

func TestTxProposalResponseFilter(t *testing.T) {
	testErrorResponse := "internal error"
	// failed if status not 200
	testPeer1 := fcmocks.NewMockPeer("Peer1", "http://peer1.com")
	testPeer2 := fcmocks.NewMockPeer("Peer2", "http://peer2.com")
	testPeer2.Status = 500
	testPeer2.ResponseMessage = testErrorResponse

	peers := []apifabclient.Peer{testPeer1, testPeer2}
	chClient := setupChannelClient(peers, t)

	_, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed for not success status")
	}
	statusError, ok := status.FromError(err)
	assert.True(t, ok, "Expected status error")
	assert.EqualValues(t, common.Status_INTERNAL_SERVER_ERROR, status.ToPeerStatusCode(statusError.Code))
	assert.Equal(t, status.EndorserServerStatus, statusError.Group)
	assert.Equal(t, testErrorResponse, statusError.Message, "Expected response message from server")

	testPeer2.Payload = []byte("wrongPayload")
	testPeer2.Status = 200
	peers = []apifabclient.Peer{testPeer1, testPeer2}
	chClient = setupChannelClient(peers, t)
	_, err = chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed for not success status")
	}
	statusError, ok = status.FromError(err)
	assert.True(t, ok, "Expected status error")
	assert.EqualValues(t, status.EndorsementMismatch, status.ToSDKStatusCode(statusError.Code))
	assert.Equal(t, status.EndorserClientStatus, statusError.Group)
	assert.Equal(t, "ProposalResponsePayloads do not match", statusError.Message, "Expected response message from server")
}

func TestQuery(t *testing.T) {

	chClient := setupChannelClient(nil, t)

	_, err := chClient.Query(chclient.Request{})
	if err == nil {
		t.Fatalf("Should have failed for empty query request")
	}

	_, err = chClient.Query(chclient.Request{Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed for empty chaincode ID")
	}

	_, err = chClient.Query(chclient.Request{ChaincodeID: "testCC", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed for empty function")
	}

	response, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err != nil {
		t.Fatalf("Failed to invoke test cc: %s", err)
	}

	if response.Payload != nil {
		t.Fatalf("Expecting nil, got %s", response.Payload)
	}

}

func TestQueryDiscoveryError(t *testing.T) {
	chClient := setupChannelClientWithError(errors.New("Test Error"), nil, nil, t)

	_, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed to query with error in discovery.GetPeers()")
	}
}

func TestQuerySelectionError(t *testing.T) {
	chClient := setupChannelClientWithError(nil, errors.New("Test Error"), nil, t)

	_, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed to query with error in selection.GetEndorsersFor ...")
	}
}

func TestQueryWithOptSync(t *testing.T) {
	chClient := setupChannelClient(nil, t)

	response, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err != nil {
		t.Fatalf("Failed to invoke test cc: %s", err)
	}

	if response.Payload != nil {
		t.Fatalf("Expecting nil, got %s", response.Payload)
	}
}

// TestQueryWithOptAsync demonstrates an example of an asynchronous query call
func TestQueryWithOptAsync(t *testing.T) {
	chClient := setupChannelClient(nil, t)
	type responseAndError struct {
		Response chclient.Response
		Error    error
	}
	notifier := make(chan responseAndError)
	go func() {
		resp, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
		notifier <- responseAndError{Response: resp, Error: err}
	}()
	resp := <-notifier
	if resp.Error != nil {
		t.Fatalf("Failed to invoke test cc: %s", resp.Error)
	}
	if resp.Response.Payload != nil {
		t.Fatalf("Expecting nil, got %s", resp.Response.Payload)
	}
}

func TestQueryWithOptTarget(t *testing.T) {
	chClient := setupChannelClient(nil, t)

	testPeer := fcmocks.NewMockPeer("Peer1", "http://peer1.com")

	peers := []apifabclient.Peer{testPeer}

	targets := peer.PeersToTxnProcessors(peers)

	response, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke",
		Args: [][]byte{[]byte("query"), []byte("b")}}, chclient.WithProposalProcessor(targets...))
	if err != nil {
		t.Fatalf("Failed to invoke test cc: %s", err)
	}

	if response.Payload != nil {
		t.Fatalf("Expecting nil, got %s", response.Payload)
	}
}

func TestExecuteTx(t *testing.T) {
	chClient := setupChannelClient(nil, t)

	_, err := chClient.Execute(chclient.Request{})
	if err == nil {
		t.Fatalf("Should have failed for empty invoke request")
	}

	_, err = chClient.Execute(chclient.Request{Fcn: "invoke", Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	if err == nil {
		t.Fatalf("Should have failed for empty chaincode ID")
	}

	_, err = chClient.Execute(chclient.Request{ChaincodeID: "testCC", Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	if err == nil {
		t.Fatalf("Should have failed for empty function")
	}

	// TODO: Test Valid Scenario with mocks
}

type customHandler struct {
	expectedPayload []byte
}

func (c *customHandler) Handle(requestContext *chclient.RequestContext, clientContext *chclient.ClientContext) {
	requestContext.Response.Payload = c.expectedPayload
}

func TestInvokeHandler(t *testing.T) {
	chClient := setupChannelClient(nil, t)

	expectedPayload := "somepayload"
	handler := &customHandler{expectedPayload: []byte(expectedPayload)}

	response, err := chClient.InvokeHandler(handler, chclient.Request{ChaincodeID: "testCC", Fcn: "move", Args: [][]byte{[]byte("a"), []byte("b"), []byte("1")}})
	if err != nil {
		t.Fatalf("Should have succeeded but got error %s", err)
	}
	if string(response.Payload) != expectedPayload {
		t.Fatalf("Expecting payload [%s] but got [%s]", expectedPayload, response.Payload)
	}
}

// customEndorsementHandler ignores the channel in the ClientContext
// and instead sends the proposal to the given channel
type customEndorsementHandler struct {
	channel apifabclient.Channel
	next    chclient.Handler
}

func (h *customEndorsementHandler) Handle(requestContext *chclient.RequestContext, clientContext *chclient.ClientContext) {
	transactionProposalResponses, txnID, err := internal.CreateAndSendTransactionProposal(h.channel,
		requestContext.Request.ChaincodeID, requestContext.Request.Fcn, requestContext.Request.Args, requestContext.Opts.ProposalProcessors, requestContext.Request.TransientMap)

	requestContext.Response.TransactionID = txnID

	if err != nil {
		requestContext.Error = err
		return
	}

	requestContext.Response.Responses = transactionProposalResponses
	if len(transactionProposalResponses) > 0 {
		requestContext.Response.Payload = transactionProposalResponses[0].ProposalResponse.GetResponse().Payload
	}

	//Delegate to next step if any
	if h.next != nil {
		h.next.Handle(requestContext, clientContext)
	}
}

func TestQueryWithCustomEndorser(t *testing.T) {
	chClient := setupChannelClient(nil, t)

	// Use the customEndorsementHandler to send the proposal to
	// the system channel instead of the channel in context

	systemChannel, err := setupChannel("")
	if err != nil {
		t.Fatalf("Error getting system channel: %s", err)
	}

	response, err := chClient.InvokeHandler(
		txnhandler.NewProposalProcessorHandler(
			&customEndorsementHandler{
				channel: systemChannel,
				next:    txnhandler.NewEndorsementValidationHandler(),
			},
		),
		chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}},
	)
	if err != nil {
		t.Fatalf("Failed to invoke test cc: %s", err)
	}

	if response.Payload != nil {
		t.Fatalf("Expecting nil, got %s", response.Payload)
	}
}

func TestExecuteTxDiscoveryError(t *testing.T) {
	chClient := setupChannelClientWithError(errors.New("Test Error"), nil, nil, t)

	_, err := chClient.Execute(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke",
		Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	if err == nil {
		t.Fatalf("Should have failed to execute tx with error in discovery.GetPeers()")
	}
}

func TestExecuteTxSelectionError(t *testing.T) {
	chClient := setupChannelClientWithError(nil, errors.New("Test Error"), nil, t)

	_, err := chClient.Execute(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke",
		Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	if err == nil {
		t.Fatalf("Should have failed to execute tx with error in selection.GetEndorserrsFor ...")
	}
}

// TestRPCErrorPropagation tests if status errors are wrapped and propagated from
// the lower level APIs to the high level channel client API
// This ensures that the status is not swallowed by calling error.Error()
func TestRPCStatusErrorPropagation(t *testing.T) {
	testErrMessage := "Test RPC Error"
	testStatus := status.New(status.EndorserClientStatus, status.ConnectionFailed.ToInt32(), testErrMessage, nil)

	testPeer1 := fcmocks.NewMockPeer("Peer1", "http://peer1.com")
	testPeer1.Error = testStatus
	chClient := setupChannelClient([]apifabclient.Peer{testPeer1}, t)

	_, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}})
	if err == nil {
		t.Fatalf("Should have failed for not success status")
	}
	statusError, ok := status.FromError(err)
	assert.True(t, ok, "Expected status error")
	assert.EqualValues(t, status.ConnectionFailed, status.ToSDKStatusCode(statusError.Code))
	assert.Equal(t, status.EndorserClientStatus, statusError.Group)
	assert.Equal(t, testErrMessage, statusError.Message, "Expected response message from server")
}

// TestOrdererStatusError ensures that status errors are propagated through
// the code execution paths from the low-level orderer broadcast APIs
func TestOrdererStatusError(t *testing.T) {
	testErrorMessage := "test error"

	testPeer1 := fcmocks.NewMockPeer("Peer1", "http://peer1.com")
	peers := []apifabclient.Peer{testPeer1}
	testOrderer1 := fcmocks.NewMockOrderer("", make(chan *apifabclient.SignedEnvelope))
	orderers := []apifabclient.Orderer{testOrderer1}
	chClient := setupChannelClientWithNodes(peers, orderers, t)
	chClient.eventHub = fcmocks.NewMockEventHub()

	mockOrderer, ok := testOrderer1.(fcmocks.MockOrderer)
	assert.True(t, ok, "Expected object to be mock orderer")
	mockOrderer.EnqueueSendBroadcastError(status.New(status.OrdererClientStatus,
		status.ConnectionFailed.ToInt32(), testErrorMessage, nil))

	_, err := chClient.Execute(chclient.Request{ChaincodeID: "test", Fcn: "invoke",
		Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	statusError, ok := status.FromError(err)
	assert.True(t, ok, "Expected status error got %+v", err)
	assert.EqualValues(t, status.ConnectionFailed, status.ToSDKStatusCode(statusError.Code))
	assert.Equal(t, status.OrdererClientStatus, statusError.Group)
	assert.Equal(t, testErrorMessage, statusError.Message, "Expected response message from server")

	chClient.Close()
}

func TestTransactionValidationError(t *testing.T) {
	validationCode := pb.TxValidationCode_BAD_RWSET
	mockEventHub := fcmocks.NewMockEventHub()
	testPeer1 := fcmocks.NewMockPeer("Peer1", "http://peer1.com")
	peers := []apifabclient.Peer{testPeer1}

	go func() {
		select {
		case callback := <-mockEventHub.RegisteredTxCallbacks:
			callback("txid", validationCode,
				status.New(status.EventServerStatus, int32(validationCode), "test", nil))
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out waiting for execute Tx to register event callback")
		}
	}()

	chClient := setupChannelClient(peers, t)
	chClient.eventHub = mockEventHub
	response, err := chClient.Execute(chclient.Request{ChaincodeID: "test", Fcn: "invoke",
		Args: [][]byte{[]byte("move"), []byte("a"), []byte("b"), []byte("1")}})
	assert.Nil(t, response.Payload, "Expected nil result on failed execute operation")
	assert.NotNil(t, err, "expected error")
	statusError, ok := status.FromError(err)
	assert.True(t, ok, "Expected status error got %+v", err)
	assert.EqualValues(t, validationCode, status.ToTransactionValidationCode(statusError.Code))
}

func TestExecuteTxWithRetries(t *testing.T) {
	testStatus := status.New(status.EndorserClientStatus, status.ConnectionFailed.ToInt32(), "test", nil)

	testPeer1 := fcmocks.NewMockPeer("Peer1", "http://peer1.com")
	testPeer1.Error = testStatus
	chClient := setupChannelClient([]apifabclient.Peer{testPeer1}, t)
	retryOpts := retry.DefaultOpts
	retryOpts.RetryableCodes = retry.ChannelClientRetryableCodes

	_, err := chClient.Query(chclient.Request{ChaincodeID: "testCC", Fcn: "invoke", Args: [][]byte{[]byte("query"), []byte("b")}},
		chclient.WithRetry(retryOpts))
	if err == nil {
		t.Fatalf("Should have failed for not success status")
	}
	assert.Equal(t, retry.DefaultOpts.Attempts, testPeer1.ProcessProposalCalls-1, "Expected peer to be called (retry attempts + 1) times")
}

func setupTestChannel() (*channel.Channel, error) {
	return setupChannel("testChannel")
}

func setupChannel(channelID string) (*channel.Channel, error) {
	ctx := setupTestContext()
	return channel.New(ctx, fcmocks.NewMockChannelCfg(channelID))
}

func setupTestContext() apifabclient.Context {
	user := fcmocks.NewMockUser("test")
	ctx := fcmocks.NewMockContext(user)
	return ctx
}

func setupTestDiscovery(discErr error, peers []apifabclient.Peer) (apifabclient.DiscoveryService, error) {

	mockDiscovery, err := txnmocks.NewMockDiscoveryProvider(discErr, peers)
	if err != nil {
		return nil, errors.WithMessage(err, "NewMockDiscoveryProvider failed")
	}

	return mockDiscovery.NewDiscoveryService("mychannel")
}

func setupTestSelection(discErr error, peers []apifabclient.Peer) (apifabclient.SelectionService, error) {

	mockSelection, err := txnmocks.NewMockSelectionProvider(discErr, peers)
	if err != nil {
		return nil, errors.WithMessage(err, "NewMockSelectinProvider failed")
	}

	return mockSelection.NewSelectionService("mychannel")
}

func setupChannelClient(peers []apifabclient.Peer, t *testing.T) *ChannelClient {

	return setupChannelClientWithError(nil, nil, peers, t)
}

func setupChannelClientWithError(discErr error, selectionErr error, peers []apifabclient.Peer, t *testing.T) *ChannelClient {

	fabCtx := setupTestContext()

	testChannel, err := setupTestChannel()
	if err != nil {
		t.Fatalf("Failed to setup test channel: %s", err)
	}

	orderer := fcmocks.NewMockOrderer("", nil)
	testChannel.AddOrderer(orderer)

	discoveryService, err := setupTestDiscovery(discErr, nil)
	if err != nil {
		t.Fatalf("Failed to setup discovery service: %s", err)
	}

	selectionService, err := setupTestSelection(selectionErr, peers)
	if err != nil {
		t.Fatalf("Failed to setup discovery service: %s", err)
	}

	ctx := Context{
		ProviderContext:  fabCtx,
		DiscoveryService: discoveryService,
		SelectionService: selectionService,
		Channel:          testChannel,
	}
	ch, err := New(ctx)
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	return ch
}

func setupChannelClientWithNodes(peers []apifabclient.Peer,
	orderers []apifabclient.Orderer, t *testing.T) *ChannelClient {

	fabCtx := setupTestContext()
	testChannel, err := setupTestChannel()
	assert.Nil(t, err, "Failed to setup test channel")

	for _, orderer := range orderers {
		err = testChannel.AddOrderer(orderer)
		assert.Nil(t, err, "Failed to add orderer %+v", orderer)
	}

	discoveryService, err := setupTestDiscovery(nil, nil)
	assert.Nil(t, err, "Failed to setup discovery service")

	selectionService, err := setupTestSelection(nil, peers)
	assert.Nil(t, err, "Failed to setup discovery service")

	ctx := Context{
		ProviderContext:  fabCtx,
		DiscoveryService: discoveryService,
		SelectionService: selectionService,
		Channel:          testChannel,
	}
	ch, err := New(ctx)
	assert.Nil(t, err, "Failed to create new channel client")

	return ch
}
