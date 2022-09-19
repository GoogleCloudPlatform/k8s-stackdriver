@Library('jenkins-helpers') _

def podTemplate = { body ->
    dockerUtils.pod() {
        spinnaker.pod() {
            node(POD_LABEL) {
                body()
            }
        }
    }
}

podTemplate {
    def destinationRepository = 'eu.gcr.io/cognitedata'
    def imageName = 'custom-metrics-stackdriver-adapter-cog-patched'
    def dockerImageTag = 'v0.12.2'
    def dockerImageName = "$destinationRepository/$imageName"

    deploySpinnakerPipelineConfigs.upload()

    stage('Checkout') {
        checkout(scm)
    }

    dir('custom-metrics-stackdriver-adapter') {
        container('docker') {
            stage('Build Docker image') {
                dockerUtils.build("$dockerImageName:$dockerImageTag")
            }
            if (env.BRANCH_NAME == 'release') {
                stage('Push image') {
                    dockerUtils.push("$dockerImageName:$dockerImageTag")
                }
            }
        }
    }
}