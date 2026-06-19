FROM node:18-alpine

WORKDIR /app
COPY package*.json ./
RUN npm ci || npm install
COPY . .

# Start the hardhat node, wait a few seconds, run the deploy script, and then keep the node alive
CMD sh -c "npx hardhat node > node.log 2>&1 & sleep 5 && npx hardhat run scripts/deploy.js --network localhost && wait"
