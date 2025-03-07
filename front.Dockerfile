FROM node:23

# Set working directory
WORKDIR /app

COPY teleops-client/package*.json ./
RUN npm install
CMD ["npm", "run", "dev"]
