require("hardhat/config");
require("@nomicfoundation/hardhat-toolbox");

// Optional: a public testnet (Sepolia) deploy target. Reads from env so no
// secrets live in the repo. Leave SEPOLIA_RPC_URL / DEPLOYER_PRIVATE_KEY unset
// to use only the local in-process / `hardhat node` networks.
const SEPOLIA_RPC_URL = process.env.SEPOLIA_RPC_URL || "";
const DEPLOYER_PRIVATE_KEY = process.env.DEPLOYER_PRIVATE_KEY || "";

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: {
    version: "0.8.24",
    settings: {
      optimizer: { enabled: true, runs: 200 },
    },
  },
  networks: {
    // `npx hardhat node` exposes this; the Go backend points ESCROW_RPC_URL here.
    localhost: {
      url: "http://127.0.0.1:8545",
    },
    ...(SEPOLIA_RPC_URL && DEPLOYER_PRIVATE_KEY
      ? {
          sepolia: {
            url: SEPOLIA_RPC_URL,
            accounts: [DEPLOYER_PRIVATE_KEY],
            chainId: 11155111,
          },
        }
      : {}),
  },
};
