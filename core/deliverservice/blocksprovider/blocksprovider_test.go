/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package blocksprovider

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	common2 "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
)

type mockMCS struct {
}

func (*mockMCS) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common2.PKIidType {
	return common2.PKIidType("pkiID")
}

func (*mockMCS) VerifyBlock(chainID common2.ChainID, signedBlock []byte) error {
	return nil
}

func (*mockMCS) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*mockMCS) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) VerifyByChannel(chainID common2.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// Used to generate a simple test case to initialize delivery
// from given block sequence number.
func makeTestCase(ledgerHeight uint64) func(*testing.T) {
	return func(t *testing.T) {
		gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}
		deliverer := &mocks.MockBlocksDeliverer{Pos: ledgerHeight}
		deliverer.MockRecv = mocks.MockRecv

		provider := &blocksProviderImpl{
			chainID: "***TEST_CHAINID***",
			gossip:  gossipServiceAdapter,
			client:  deliverer,
			mcs:     &mockMCS{},
		}

		ready := make(chan struct{})
		go func() {
			go provider.DeliverBlocks()
			// Send notification
			ready <- struct{}{}
		}()

		time.Sleep(time.Duration(10) * time.Millisecond)
		provider.Stop()

		select {
		case <-ready:
			{
				// Check that all blocks received eventually get gossiped and locally committed
				assert.True(t, deliverer.RecvCnt == gossipServiceAdapter.AddPayloadsCnt)
				select {
				case <-gossipServiceAdapter.GossipBlockDisseminations:
				case <-time.After(time.Second):
					assert.Fail(t, "Didn't gossip a block within a timely manner")
				}
				return
			}
		case <-time.After(time.Duration(1) * time.Second):
			{
				t.Fatal("Test hasn't finished in timely manner, failing.")
			}
		}
	}
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromTheOldest(t *testing.T) {
	makeTestCase(uint64(0))(t)
}

/*
   Test to check whenever blocks provider starts calling new blocks from the
   oldest and that eventually it terminates after the Stop method has been called.
*/
func TestBlocksProviderImpl_GetBlockFromSpecified(t *testing.T) {
	makeTestCase(uint64(101))(t)
}

func TestBlocksProvider_CheckTerminationDeliveryResponseStatus(t *testing.T) {

	tmp := struct{ mocks.MockBlocksDeliverer }{}

	// Making mocked Recv() function to return DeliverResponse_Status to force block
	// provider to fail and exit, cheking that in that case to block was actually
	// delivered.
	tmp.MockRecv = func(mock *mocks.MockBlocksDeliverer) (*orderer.DeliverResponse, error) {
		return &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Status{
				Status: common.Status_SUCCESS,
			},
		}, nil
	}

	gossipServiceAdapter := &mocks.MockGossipServiceAdapter{}
	provider := &blocksProviderImpl{
		chainID: "***TEST_CHAINID***",
		gossip:  gossipServiceAdapter,
		client:  &tmp,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	ready := make(chan struct{})
	go func() {
		provider.DeliverBlocks()
		wg.Done()
		// Send notification
		ready <- struct{}{}
	}()

	time.Sleep(time.Duration(10) * time.Millisecond)
	provider.Stop()

	select {
	case <-ready:
		{
			assert.Equal(t, int32(1), tmp.RecvCnt)
			// No payload should commit locally
			assert.Equal(t, int32(0), gossipServiceAdapter.AddPayloadsCnt)
			// No payload should be transfered to other peers
			select {
			case <-gossipServiceAdapter.GossipBlockDisseminations:
				assert.Fail(t, "Gossiped block but shouldn't have")
			case <-time.After(time.Second):
			}
			return
		}
	case <-time.After(time.Duration(1) * time.Second):
		{
			t.Fatal("Test hasn't finished in timely manner, failing.")
		}
	}
}
