# Image URL to use all building/pushing image targets
IMG ?= notebook-ssh-controller
VER ?= 1.0
# Build the docker image
.PHONY: docker-build
docker-build:
	docker build -t cguaicadmin/${IMG}:${VER} .

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push cguaicadmin/${IMG}:${VER}

# Deploy to the cluster
.PHONY: deploy
deploy:
	kubectl apply -k config/manager

# Undeploy from the cluster
.PHONY: undeploy
undeploy:
	kubectl delete -k config/manager

# Run tests
.PHONY: test
test:
	go test ./... -coverprofile cover.out

# Clean up
.PHONY: clean
clean:
	rm -f manager cover.out