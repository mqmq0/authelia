package suites

import (
	"fmt"
	//TODO(nightah): Remove when turning off Travis
	"os"
	"time"

	"github.com/authelia/authelia/internal/utils"
	log "github.com/sirupsen/logrus"
)

var kubernetesSuiteName = "Kubernetes"

func init() {
	kind := Kind{}
	kubectl := Kubectl{}

	setup := func(suitePath string) error {
		cmd := utils.Shell("docker-compose -f docker-compose.yml -f example/compose/kind/docker-compose.yml build")
		if err := cmd.Run(); err != nil {
			return err
		}

		cmd = utils.Shell("docker build -t nginx-backend example/compose/nginx/backend")
		if err := cmd.Run(); err != nil {
			return err
		}

		exists, err := kind.ClusterExists()

		if err != nil {
			return err
		}

		if exists {
			log.Debug("Kubernetes cluster already exists")
		} else {
			err = kind.CreateCluster()

			if err != nil {
				return err
			}
		}

		log.Debug("Building authelia:dist image...")
		//TODO(nightah): Remove when turning off Travis
		travis := os.Getenv("TRAVIS")
		if travis == "true" {
			if err := utils.Shell("authelia-scripts docker build").Run(); err != nil {
				return err
			}
		} else {
			if err := utils.Shell("authelia-scripts docker build --arch=CI").Run(); err != nil {
				return err
			}
		}
		//TODO(nightah): Remove when turning off Travis

		log.Debug("Loading images into Kubernetes container...")
		if err = loadDockerImages(); err != nil {
			return err
		}

		log.Debug("Starting Kubernetes dashboard...")
		if err := kubectl.StartDashboard(); err != nil {
			return err
		}

		log.Debug("Deploying thirdparties...")
		if err = kubectl.DeployThirdparties(); err != nil {
			return err
		}

		log.Debug("Waiting for services to be ready...")
		if err := waitAllPodsAreReady(5 * time.Minute); err != nil {
			return err
		}

		log.Debug("Deploying Authelia...")
		if err = kubectl.DeployAuthelia(); err != nil {
			return err
		}

		log.Debug("Waiting for services to be ready...")
		if err := waitAllPodsAreReady(2 * time.Minute); err != nil {
			return err
		}

		log.Debug("Starting proxy...")
		if err := kubectl.StartProxy(); err != nil {
			return err
		}
		return nil
	}

	teardown := func(suitePath string) error {
		kubectl.StopDashboard()
		kubectl.StopProxy()
		return kind.DeleteCluster()
	}

	GlobalRegistry.Register(kubernetesSuiteName, Suite{
		SetUp:           setup,
		SetUpTimeout:    12 * time.Minute,
		TestTimeout:     2 * time.Minute,
		TearDown:        teardown,
		TearDownTimeout: 2 * time.Minute,
		Description:     "This suite has been created to test Authelia in a Kubernetes context and using nginx as the ingress controller.",
	})
}

func loadDockerImages() error {
	kind := Kind{}
	images := []string{"authelia:dist", "nginx-backend"}

	for _, image := range images {
		err := kind.LoadImage(image)

		if err != nil {
			return err
		}
	}

	return nil
}

func waitAllPodsAreReady(timeout time.Duration) error {
	kubectl := Kubectl{}
	// Wait in case the deployment has just been done and some services do not appear in kubectl logs.
	time.Sleep(1 * time.Second)
	fmt.Println("Check services are running")
	if err := kubectl.WaitPodsReady(timeout); err != nil {
		return err
	}
	fmt.Println("All pods are ready")
	return nil
}
