package test

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-hclog"
	"github.com/phayes/freeport"
	bind "github.com/supranational/blst/bindings/go"
	blst "github.com/supranational/blst/bindings/go"
	"io/fs"
	"io/ioutil"
	"math/big"
	"os"
	"os/exec"
	"time"
)

var (
	oracleConfigDir    = "./oracle_config"
	nodeKeyDir         = "./autonity_l1_config/nodekeys"
	keyStoreDir        = "./autonity_l1_config/keystore"
	defaultHost        = "127.0.0.1"
	defaultPlugDir     = "./plugins/template_plugins"
	forexPlugDir       = "./plugins/forex_plugins"
	cryptoPlugDir      = "./plugins/crypto_plugins"
	binancePlugDir     = "./plugins/production_plugins"
	simulatorPlugDir   = "./plugins/simulator_plugins"
	mixPluginDir       = "./plugins/mix_plugins"
	defaultGenesis     = "./autonity_l1_config/genesis_template.json"
	defaultPassword    = "test"
	generatedGenesis   = "./autonity_l1_config/genesis_gen.json"
	defaultDataDirRoot = "./autonity_l1_config/nodes"
	defaultPlugConf    = "./plugins/plugins-conf.yml"

	defaultBondedStake = new(big.Int).SetUint64(1000)

	numberOfKeys              = 10
	numberOfValidators        = 4
	numberOfPortsForBindNodes = 3
)

// ErrZeroKey describes an error due to a zero secret key.
var ErrZeroKey = errors.New("generated secret key is zero")

type AutonityContractGenesis struct {
	Bytecode         hexutil.Bytes  `json:"bytecode,omitempty" toml:",omitempty"`
	ABI              *abi.ABI       `json:"abi,omitempty" toml:",omitempty"`
	MinBaseFee       uint64         `json:"minBaseFee"`
	EpochPeriod      uint64         `json:"epochPeriod"`
	UnbondingPeriod  uint64         `json:"unbondingPeriod"`
	BlockPeriod      uint64         `json:"blockPeriod"`
	MaxCommitteeSize uint64         `json:"maxCommitteeSize"`
	Operator         common.Address `json:"operator"`
	Treasury         common.Address `json:"treasury"`
	TreasuryFee      uint64         `json:"treasuryFee"`
	DelegationRate   uint64         `json:"delegationRate"`
	Validators       []*Validator   `json:"validators"`
}

type AccountabilityGenesis struct {
	InnocenceProofSubmissionWindow uint64 `json:"innocenceProofSubmissionWindow"`
	// Slashing parameters
	BaseSlashingRateLow   uint64 `json:"baseSlashingRateLow"`
	BaseSlashingRateMid   uint64 `json:"baseSlashingRateMid"`
	CollusionFactor       uint64 `json:"collusionFactor"`
	HistoryFactor         uint64 `json:"historyFactor"`
	JailFactor            uint64 `json:"jailFactor"`
	SlashingRatePrecision uint64 `json:"slashingRatePrecision"`
}

// OracleContractGenesis Autonity contract config. It'is used for deployment.
type OracleContractGenesis struct {
	Bytecode   hexutil.Bytes `json:"bytecode,omitempty" toml:",omitempty"`
	ABI        *abi.ABI      `json:"abi,omitempty" toml:",omitempty"`
	Symbols    []string      `json:"symbols"`
	VotePeriod uint64        `json:"votePeriod"`
}

type AsmConfig struct {
	ACUContractConfig           *AcuContractGenesis           `json:"acu,omitempty"`
	StabilizationContractConfig *StabilizationContractGenesis `json:"stabilization,omitempty"`
	SupplyControlConfig         *SupplyControlGenesis         `json:"supplyControl,omitempty"`
}

type AcuContractGenesis struct {
	Symbols    []string
	Quantities []uint64
	Scale      uint64
}

type StabilizationContractGenesis struct {
	BorrowInterestRate        *math.HexOrDecimal256
	LiquidationRatio          *math.HexOrDecimal256
	MinCollateralizationRatio *math.HexOrDecimal256
	MinDebtRequirement        *math.HexOrDecimal256
	TargetPrice               *math.HexOrDecimal256
}

type SupplyControlGenesis struct {
	InitialAllocation *math.HexOrDecimal256
}

// EthashConfig is the consensus engine configs for proof-of-work based sealing.
type EthashConfig struct{}

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)

	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   // TheDAO hard-fork switch block (nil = no fork)
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"` // Whether the nodes supports or opposes the DAO hard-fork

	// EIP150 implements the Gas price changes (https://github.com/ethereum/EIPs/issues/150)
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` // EIP150 HF block (nil = no fork)
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  // EIP150 HF hash (needed for header only clients as only gas pricing changed)

	EIP155Block *big.Int `json:"eip155Block,omitempty"` // EIP155 HF block
	EIP158Block *big.Int `json:"eip158Block,omitempty"` // EIP158 HF block

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      // Byzantium switch block (nil = no fork, 0 = already on byzantium)
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` // Constantinople switch block (nil = no fork, 0 = already activated)
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`     // Petersburg switch block (nil = same as Constantinople)
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`       // Istanbul switch block (nil = no fork, 0 = already on istanbul)
	MuirGlacierBlock    *big.Int `json:"muirGlacierBlock,omitempty"`    // Eip-2384 (bomb delay) switch block (nil = no fork, 0 = already activated)
	BerlinBlock         *big.Int `json:"berlinBlock,omitempty"`         // Berlin switch block (nil = no fork, 0 = already on berlin)
	LondonBlock         *big.Int `json:"londonBlock,omitempty"`         // London switch block (nil = no fork, 0 = already on london)
	ArrowGlacierBlock   *big.Int `json:"arrowGlacierBlock,omitempty"`   // Eip-4345 (bomb delay) switch block (nil = no fork, 0 = already activated)
	MergeForkBlock      *big.Int `json:"mergeForkBlock,omitempty"`      // EIP-3675 (TheMerge) switch block (nil = no fork, 0 = already in merge proceedings)

	// TerminalTotalDifficulty is the amount of total difficulty reached by
	// the network that triggers the consensus upgrade.
	TerminalTotalDifficulty *big.Int `json:"terminalTotalDifficulty,omitempty"`

	// Various consensus engines
	Ethash                 *EthashConfig            `json:"ethash,omitempty"`
	AutonityContractConfig *AutonityContractGenesis `json:"autonity,omitempty"`
	AccountabilityConfig   *AccountabilityGenesis   `json:"accountability,omitempty"`
	OracleContractConfig   *OracleContractGenesis   `json:"oracle,omitempty"`

	ASM AsmConfig `json:"asm,omitempty"`

	// true if run in test-mode, false by default
	TestMode bool `json:"testMode,omitempty"`
}

type Validator struct {
	Treasury      common.Address `json:"treasury"`
	Enode         string         `json:"enode"`
	OracleAddress common.Address `json:"oracleAddress"`
	BondedStake   *big.Int       `json:"bondedStake"`
	ConsensusKey  string         `json:"consensusKey"`
}

type DataSimulator struct {
	Command    *exec.Cmd
	SimulateTM int
}

func (s *DataSimulator) Start() {
	err := s.Command.Run()
	if err != nil {
		log.Error("start data simulator failed", "error", err.Error())
	}
}

func (s *DataSimulator) Stop() {
	err := s.Command.Process.Kill()
	if err != nil {
		log.Error("stop data simulator failed", "error", err.Error())
	}
}

func (s *DataSimulator) GenCMD() {
	c := exec.Command("./simulator", fmt.Sprintf("-sim_timeout=%d", s.SimulateTM))
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	s.Command = c
}

type Oracle struct {
	Key        *Key
	PluginDir  string
	PluginConf string
	OracleConf string
	Host       string
	Command    *exec.Cmd
}

// Start starts the process and wait until it exists, the caller should use a go routine to invoke it.
func (o *Oracle) Start() {
	err := o.Command.Run()
	if err != nil {
		panic(err)
	}
}

func (o *Oracle) Stop() {
	err := o.Command.Process.Kill()
	if err != nil {
		log.Error("stop oracle client failed", "error", err.Error())
	}
	err = os.Remove(o.OracleConf)
	if err != nil {
		log.Error("clean up oracle server config failed", "error", err.Error())
	}
}

func (o *Oracle) ConfigOracleServer(wsEndpoint string) {
	// write config to config file:
	f, err := os.Create(o.OracleConf)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("ws %s\nkey.file %s\nkey.password %s\nplugin.dir %s\nlog.level %d\nplugin.conf %s\n",
		wsEndpoint, o.Key.KeyFile, o.Key.Password, o.PluginDir, hclog.Debug, o.PluginConf))
	if err != nil {
		panic(err)
	}

	// prepare cli command
	c := exec.Command("./autoracle", fmt.Sprintf("-config=%s", o.OracleConf))
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	o.Command = c
}

// RandBLSKey creates a new private key using a random method provided as an io.Reader.
func RandBLSKey() (SecretKey, error) {
	// Generate 32 bytes of randomness
	var ikm [32]byte
	_, err := rand.Read(ikm[:])
	if err != nil {
		return nil, err
	}
	// Defensive check, that we have not generated a secret key,
	secKey := &bls12SecretKey{blst.KeyGen(ikm[:])}

	if IsZero(secKey.Marshal()) {
		return nil, ErrZeroKey
	}

	return secKey, nil
}

// SecretKey represents a BLS secret or private key.
type SecretKey interface {
	Marshal() []byte
	PublicKey() []byte
}

// bls12SecretKey used in the BLS signature scheme.
type bls12SecretKey struct {
	p *blst.SecretKey
}

// Marshal a secret key into a LittleEndian byte slice.
func (s *bls12SecretKey) Marshal() []byte {
	keyBytes := s.p.Serialize()
	return keyBytes
}

func (s *bls12SecretKey) PublicKey() []byte {
	pub := &BlsPublicKey{p: new(blstPublicKey).From(s.p)}
	return pub.p.Compress()
}

// BlsPublicKey used in the BLS signature scheme.
type BlsPublicKey struct {
	p *blstPublicKey
}

type blstPublicKey = bind.P1Affine

// IsZero checks if the secret key is a zero key. We don't rely on the CGO to refer to the type of C.blst_scalar which
// is implemented in C to initialize the memory bits of C.blst_scalar to be zero. It is better for go binder to
// check if all the bytes of the secret key are zero.
func IsZero(sKey []byte) bool {
	b := byte(0)
	for _, s := range sKey {
		b |= s
	}
	return subtle.ConstantTimeByteEq(b, 0) == 1
}

type Key struct {
	KeyFile          string
	AutonityKeysFile string
	ConsensusKey     string
	Password         string
	Key              *keystore.Key
}

type L1Node struct {
	EnableLog bool
	DataDir   string
	NodeKey   *Key
	Host      string
	P2PPort   int
	ACNPort   int
	WSPort    int
	Command   *exec.Cmd
	Validator *Validator
}

func (n *L1Node) GenCMD(genesisFile string) {
	c := exec.Command("./autonity",
		"--ipcdisable", "--datadir", n.DataDir, "--genesis", genesisFile, "--autonitykeys", n.NodeKey.AutonityKeysFile, "--ws",
		"--ws.addr", n.Host, "--ws.port", fmt.Sprintf("%d", n.WSPort), "--consensus.port", fmt.Sprintf("%d", n.ACNPort),
		"--ws.api", "tendermint,eth,web3,admin,debug,miner,personal,txpool,net", "--syncmode", "full", "--miner.gaslimit",
		"100000000", "--miner.threads", fmt.Sprintf("%d", 1), "--port", fmt.Sprintf("%d", n.P2PPort))

	// enable logging in the standard outputs.
	if n.EnableLog {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}

	n.Command = c
}

// Start starts the process and wait until it exists, the caller should use a go routine to invoke it.
func (n *L1Node) Start() {
	err := n.Command.Run()
	if err != nil {
		log.Error("L1 node start error", "error", err)
	}
}

func (n *L1Node) Stop() {
	if err := n.Command.Process.Kill(); err != nil {
		log.Warn("stop autonity client failed", "error", err.Error())
	}
	if err := os.RemoveAll(n.DataDir); err != nil {
		log.Warn("cleanup autonity client data DIR failed", "error", err.Error())
	}
}

type NetworkConfig struct {
	EnableL1Logs    bool
	Symbols         []string
	VotePeriod      uint64
	PluginDIRs      []string // different oracle can have different plugins configured.
	SimulateTimeout int      // to simulate timeout in seconds at data source simulator when processing http request.
}

type Network struct {
	EnableL1Logs bool
	GenesisFile  string
	OperatorKey  *Key
	TreasuryKey  *Key
	L1Nodes      []*L1Node
	L2Nodes      []*Oracle
	Simulator    *DataSimulator
	Symbols      []string
	VotePeriod   uint64
	PluginDirs   []string // different oracle can have different plugins configured.
	PluginConf   []string // different oracle can have different plugin conf.
}

func (net *Network) genGenesisFile() error {
	srcGenesis := defaultGenesis
	dstGenesis := generatedGenesis

	var vals []*Validator
	for _, n := range net.L1Nodes {
		vals = append(vals, n.Validator)
	}

	err := makeGenesisConfig(srcGenesis, dstGenesis, vals, net)
	if err != nil {
		return err
	}
	net.GenesisFile = dstGenesis
	return nil
}

func (net *Network) Start() {
	if net.Simulator != nil {
		net.Simulator.GenCMD()
		go net.Simulator.Start()
	}

	for _, n := range net.L1Nodes {
		n.GenCMD(net.GenesisFile)
		go n.Start()
	}

	time.Sleep(5 * time.Second)

	for i, n := range net.L2Nodes {
		n.ConfigOracleServer(fmt.Sprintf("ws://%s:%d", net.L1Nodes[i].Host, net.L1Nodes[i].WSPort))
		go n.Start()
	}
}

func (net *Network) ResetL1Node(index int) {
	for i, n := range net.L1Nodes {
		if i == index {
			n.Stop()
			n.GenCMD(net.GenesisFile)
			go n.Start()
		}
	}
}

func (net *Network) Stop() {
	for _, n := range net.L1Nodes {
		n.Stop()
	}

	for _, n := range net.L2Nodes {
		n.Stop()
	}

	if net.Simulator != nil {
		net.Simulator.Stop()
	}
}

// create with a four-nodes autonity l1 network for the integration of oracle service, with each of validator bind with
// an oracle node.
func createNetwork(netConf *NetworkConfig, numOfValidators int) (*Network, error) {
	keys, err := loadKeys(keyStoreDir, defaultPassword)
	if err != nil {
		return nil, err
	}

	if len(keys) != numberOfKeys {
		panic("keystore does not contains enough key for testbed")
	}

	var pluginConfs = []string{defaultPlugConf, defaultPlugConf, defaultPlugConf, defaultPlugConf}
	var pluginDIRs = []string{defaultPlugDir, defaultPlugDir, defaultPlugDir, defaultPlugDir}

	var simulator *DataSimulator
	for i, d := range netConf.PluginDIRs {
		if i >= numOfValidators {
			break
		}
		if len(d) != 0 {
			pluginDIRs[i] = d
			if (d == simulatorPlugDir || d == mixPluginDir) && simulator == nil {
				simulator = &DataSimulator{SimulateTM: netConf.SimulateTimeout}
			}
		}
	}

	var network = &Network{
		EnableL1Logs: netConf.EnableL1Logs,
		OperatorKey:  keys[0],
		TreasuryKey:  keys[1],
		Symbols:      netConf.Symbols,
		VotePeriod:   netConf.VotePeriod,
		PluginDirs:   pluginDIRs,
		PluginConf:   pluginConfs,
		Simulator:    simulator,
	}

	freePorts, err := getFreePost(numOfValidators * numberOfPortsForBindNodes)
	if err != nil {
		return nil, err
	}

	network, err = configNetwork(network, keys[2:], freePorts, numOfValidators)
	if err != nil {
		return nil, err
	}

	network.Start()

	return network, nil
}

func configNetwork(network *Network, freeKeys []*Key, freePorts []int, nodes int) (*Network, error) {

	for i := 0; i < nodes; i++ {
		// allocate a key and a port for l2Node client,
		var l2Node = &Oracle{
			Key:        freeKeys[i*2],
			PluginDir:  network.PluginDirs[i],
			PluginConf: network.PluginConf[i],
			OracleConf: fmt.Sprintf("%s/oracle-server.config%d", oracleConfigDir, i),
			Host:       defaultHost,
		}

		// allocate a key and 2 ports for l1 validator client,
		var l1Node = &L1Node{
			EnableLog: network.EnableL1Logs,
			DataDir:   fmt.Sprintf("%s/node_%d/data", defaultDataDirRoot, i),
			NodeKey:   freeKeys[i*2+1],
			Host:      defaultHost,
			P2PPort:   freePorts[i*3],
			WSPort:    freePorts[i*3+1],
			ACNPort:   freePorts[i*3+2],
		}

		var validator = &Validator{
			Treasury:      l1Node.NodeKey.Key.Address,
			Enode:         genEnode(&l1Node.NodeKey.Key.PrivateKey.PublicKey, l1Node.Host, l1Node.P2PPort, l1Node.ACNPort),
			OracleAddress: crypto.PubkeyToAddress(l2Node.Key.Key.PrivateKey.PublicKey),
			BondedStake:   defaultBondedStake,
			ConsensusKey:  l1Node.NodeKey.ConsensusKey,
		}

		l1Node.Validator = validator

		// clean up legacy data in the data DIR for the test framework.
		if err := os.RemoveAll(l1Node.DataDir); err != nil {
			log.Warn("Cannot cleanup legacy data for the test framework", "err", err.Error())
		}

		network.L1Nodes = append(network.L1Nodes, l1Node)
		network.L2Nodes = append(network.L2Nodes, l2Node)
	}

	if err := network.genGenesisFile(); err != nil {
		return nil, err
	}

	return network, nil
}

func makeGenesisConfig(srcTemplate string, dstFile string, vals []*Validator, net *Network) error {
	file, err := os.Open(srcTemplate)
	if err != nil {
		return err
	}

	defer file.Close()

	genesis := new(Genesis)
	if err = json.NewDecoder(file).Decode(genesis); err != nil {
		return err
	}
	genesis.Config.AutonityContractConfig.Operator = net.OperatorKey.Key.Address
	genesis.Config.AutonityContractConfig.Treasury = net.TreasuryKey.Key.Address
	genesis.Config.AutonityContractConfig.Validators = append(genesis.Config.AutonityContractConfig.Validators, vals...)

	genesis.Config.OracleContractConfig.Symbols = net.Symbols
	genesis.Config.OracleContractConfig.VotePeriod = net.VotePeriod

	jsonData, err := json.MarshalIndent(genesis, "", " ")
	if err != nil {
		return err
	}

	if err = os.WriteFile(dstFile, jsonData, 0644); err != nil {
		return err
	}

	return nil
}

// load all keys from keystore with the corresponding password.
func loadKeys(kStore string, password string) ([]*Key, error) {
	files, err := listDir(kStore)
	if err != nil {
		return nil, err
	}

	var keys []*Key
	for _, f := range files {
		keyFile := fmt.Sprintf(fmt.Sprintf("%s/%s", kStore, f.Name()))
		keyJson, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}

		key, err := keystore.DecryptKey(keyJson, password)
		if err != nil {
			return nil, err
		}

		ecdsaKey := hex.EncodeToString(crypto.FromECDSA(key.PrivateKey))
		blsKey, err := RandBLSKey()
		if err != nil {
			return nil, err
		}
		rawKeyFile := fmt.Sprintf("%s/%s", nodeKeyDir, key.Address)
		if err := os.WriteFile(rawKeyFile, []byte(ecdsaKey+hex.EncodeToString(blsKey.Marshal())), 0666); err != nil {
			return nil, err
		}

		consensusKey := "0x" + hex.EncodeToString(blsKey.PublicKey())
		var k = &Key{Key: key, KeyFile: keyFile, Password: password, AutonityKeysFile: rawKeyFile, ConsensusKey: consensusKey}
		keys = append(keys, k)
	}

	return keys, nil
}

// generate enode url
func genEnode(key *ecdsa.PublicKey, host string, p2pPort int, acnPort int) string {
	pub := fmt.Sprintf("%x", crypto.FromECDSAPub(key)[1:])
	return fmt.Sprintf("enode://%s@%s:%d?acn=%s:%d", pub, host, p2pPort, host, acnPort)
}

// get free ports from current system
func getFreePost(numOfPort int) ([]int, error) {
	return freeport.GetFreePorts(numOfPort)
}

func listDir(pluginDIR string) ([]fs.FileInfo, error) {
	var plugins []fs.FileInfo

	files, err := ioutil.ReadDir(pluginDIR)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		plugins = append(plugins, file)
	}
	return plugins, nil
}
