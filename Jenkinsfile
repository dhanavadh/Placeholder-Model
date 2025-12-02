pipeline {
    agent any

    environment {
        GO111MODULE = 'on'
        SERVICE_NAME = 'placeholder-model'
        BINARY_NAME = 'placeholder-model'
    }

    triggers {
        pollSCM('* * * * *')
    }

    stages {
        stage('Clean Workspace') {
            steps {
                cleanWs()
            }
        }

        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Install & Test') {
            steps {
                sh 'go mod download'
                sh 'go test -v ./...'
            }
        }

        stage('Build') {
            steps {
                // Note: entry point is cmd/server/main.go for this service
                sh "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o ${BINARY_NAME} ./cmd/server/main.go"
            }
        }

        stage('Deploy') {
            steps {
                script {
                    if (!env.SERVER_IP || !env.SSH_CRED_ID || !env.SERVER_USER || !env.REMOTE_DIR) {
                        error "Missing required environment variables: SERVER_IP, SSH_CRED_ID, SERVER_USER, REMOTE_DIR"
                    }

                    sshagent([SSH_CRED_ID]) {
                        // Prepare service file
                        sh "sed 's/REPLACE_ME_USER/${SERVER_USER}/g' ${SERVICE_NAME}.service > ${SERVICE_NAME}.service.tmp"

                        // Upload service file
                        sh "scp -o StrictHostKeyChecking=no ${SERVICE_NAME}.service.tmp ${SERVER_USER}@${SERVER_IP}:/tmp/${SERVICE_NAME}.service"
                        sh "ssh -o StrictHostKeyChecking=no ${SERVER_USER}@${SERVER_IP} 'sudo mv /tmp/${SERVICE_NAME}.service /etc/systemd/system/${SERVICE_NAME}.service && sudo systemctl daemon-reload'"

                        // Upload binary
                        sh "scp -o StrictHostKeyChecking=no ${BINARY_NAME} ${SERVER_USER}@${SERVER_IP}:${REMOTE_DIR}/"

                        // Inject env vars from Jenkins credentials (using shared credentials)
                        // Internal service - using local storage (no GCS needed)
                        withCredentials([
                            string(credentialsId: 'shared-db-host', variable: 'DB_HOST'),
                            string(credentialsId: 'shared-db-port', variable: 'DB_PORT'),
                            string(credentialsId: 'shared-db-user', variable: 'DB_USER'),
                            string(credentialsId: 'shared-db-password', variable: 'DB_PASSWORD'),
                            string(credentialsId: 'placeholder-db-name', variable: 'DB_NAME'),
                            string(credentialsId: 'placeholder-gotenberg-url', variable: 'GOTENBERG_URL')
                        ]) {
                            sh """
                                ssh -o StrictHostKeyChecking=no ${SERVER_USER}@${SERVER_IP} 'mkdir -p ${REMOTE_DIR}/storage && cat > ${REMOTE_DIR}/.env << EOF
SERVER_PORT=3004
ENVIRONMENT=production
BASE_URL=http://localhost:3004
STORAGE_TYPE=local
STORAGE_LOCAL_PATH=${REMOTE_DIR}/storage
DB_HOST=${DB_HOST}
DB_PORT=${DB_PORT}
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=${DB_NAME}
GOTENBERG_URL=${GOTENBERG_URL}
EOF'
                            """
                        }

                        // Restart service
                        sh "ssh -o StrictHostKeyChecking=no ${SERVER_USER}@${SERVER_IP} 'sudo systemctl restart ${SERVICE_NAME}'"

                        // Cleanup
                        sh "rm ${SERVICE_NAME}.service.tmp"
                    }
                }
            }
        }
    }

    post {
        success { echo 'Pipeline completed successfully.' }
        failure { echo 'Pipeline failed.' }
    }
}
