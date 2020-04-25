package consensus_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-filecoin/internal/pkg/block"
	"github.com/filecoin-project/go-filecoin/internal/pkg/cborutil"
	"github.com/filecoin-project/go-filecoin/internal/pkg/consensus"
	"github.com/filecoin-project/go-filecoin/internal/pkg/constants"
	"github.com/filecoin-project/go-filecoin/internal/pkg/crypto"
	"github.com/filecoin-project/go-filecoin/internal/pkg/repo"
	"github.com/filecoin-project/go-filecoin/internal/pkg/state"
	tf "github.com/filecoin-project/go-filecoin/internal/pkg/testhelpers/testflags"
	"github.com/filecoin-project/go-filecoin/internal/pkg/types"
	gengen "github.com/filecoin-project/go-filecoin/tools/gengen/util"
)

func TestTotal(t *testing.T) {
	tf.UnitTest(t)

	ctx := context.Background()
	numCommittedSectors := uint64(19)
	numMiners := 3
	kis := types.MustGenerateBLSKeyInfo(numMiners, 0)

	cst, _, root := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis)

	table := consensus.NewPowerTableView(state.NewView(cst, root), state.NewView(cst, root))
	actual, err := table.Total(ctx)
	require.NoError(t, err)

	expected := abi.NewStoragePower(int64(uint64(constants.DevSectorSize) * numCommittedSectors * uint64(numMiners)))
	assert.True(t, expected.Equals(actual))
}

func TestMiner(t *testing.T) {
	tf.UnitTest(t)
	t.Skip("power setting has become more complex and gengen setup and testing expectations need to reflect that")

	ctx := context.Background()
	kis := types.MustGenerateBLSKeyInfo(1, 0)

	numCommittedSectors := uint64(10)
	cst, addrs, root := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis)
	addr := addrs[0]

	table := consensus.NewPowerTableView(state.NewView(cst, root), state.NewView(cst, root))
	actual, err := table.MinerClaim(ctx, addr)
	require.NoError(t, err)

	expected := abi.NewStoragePower(int64(uint64(constants.DevSectorSize) * numCommittedSectors))
	assert.True(t, expected.Equals(actual))
	assert.Equal(t, expected, actual)
}

func TestNoPowerAfterSlash(t *testing.T) {
	tf.UnitTest(t)
	// setup lookback state with 3 miners
	ctx := context.Background()
	numCommittedSectors := uint64(19)
	numMiners := 3
	kis := types.MustGenerateBLSKeyInfo(numMiners, 0)
	cstPower, addrsPower, rootPower := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis)
	cstFaults, _, rootFaults := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis[0:2]) // drop the third key
	table := consensus.NewPowerTableView(state.NewView(cstPower, rootPower), state.NewView(cstFaults, rootFaults))

	// verify that faulted miner claim is 0 power
	claim, err := table.MinerClaim(ctx, addrsPower[2])
	require.NoError(t, err)
	assert.Equal(t, abi.NewStoragePower(0), claim)
}

func TestTotalPowerUnaffectedBySlash(t *testing.T) {
	tf.UnitTest(t)
	ctx := context.Background()
	numCommittedSectors := uint64(19)
	numMiners := 3
	kis := types.MustGenerateBLSKeyInfo(numMiners, 0)
	cstPower, _, rootPower := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis)
	cstFaults, _, rootFaults := requireMinerWithNumCommittedSectors(ctx, t, numCommittedSectors, kis[0:2]) // drop the third key
	table := consensus.NewPowerTableView(state.NewView(cstPower, rootPower), state.NewView(cstFaults, rootFaults))

	// verify that faulted miner claim is 0 power
	total, err := table.Total(ctx)
	require.NoError(t, err)
	expected := abi.NewStoragePower(int64(uint64(constants.DevSectorSize) * numCommittedSectors * uint64(numMiners)))

	assert.Equal(t, expected, total)
}

func requireMinerWithNumCommittedSectors(ctx context.Context, t *testing.T, numCommittedSectors uint64, ownerKeys []crypto.KeyInfo) (*cborutil.IpldStore, []address.Address, cid.Cid) {
	r := repo.NewInMemoryRepo()
	bs := bstore.NewBlockstore(r.Datastore())
	cst := cborutil.NewIpldStore(bs)
	numMiners := len(ownerKeys)
	minerConfigs := make([]*gengen.CreateStorageMinerConfig, numMiners)
	for i := 0; i < numMiners; i++ {
		commCfgs, err := gengen.MakeCommitCfgs(int(numCommittedSectors))
		require.NoError(t, err)
		minerConfigs[i] = &gengen.CreateStorageMinerConfig{
			CommittedSectors: commCfgs,
			SectorSize:       constants.DevSectorSize,
		}
	}

	// Tedious conversion to pointer type
	genKis := make([]*crypto.KeyInfo, numMiners)
	for i, ki := range ownerKeys {
		genKis[i] = &ki
	}

	// set up genesis block containing some miners with non-zero power
	genCfg := &gengen.GenesisCfg{
		ProofsMode: types.TestProofsMode,
		Miners:     minerConfigs,
		Network:    "ptvtest",
		ImportKeys: genKis,
	}

	info, err := gengen.GenGen(ctx, genCfg, bs)
	require.NoError(t, err)

	var genesis block.Block
	require.NoError(t, cst.Get(ctx, info.GenesisCid, &genesis))
	retAddrs := make([]address.Address, numMiners)
	for i := 0; i < numMiners; i++ {
		retAddrs[i] = info.Miners[i].Address
	}
	return cst, retAddrs, genesis.StateRoot.Cid
}
