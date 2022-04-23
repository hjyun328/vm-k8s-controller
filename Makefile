.PHONY: deploy

build:
	docker build --tag ${IMAGE} .

push:
	docker push ${IMAGE}

deploy:
	sed 's|\$${IMAGE}|${IMAGE}|g' deploy/deployment.yaml.template > deploy/deployment.yaml
	kubectl apply -f deploy/secret.yaml
	kubectl apply -f deploy/crd.yaml
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/webhook.yaml
	kubectl apply -f deploy/deployment.yaml

clean:
	kubectl delete -f deploy/deployment.yaml
	kubectl delete -f deploy/secret.yaml
	kubectl delete -f deploy/crd.yaml
	kubectl delete -f deploy/rbac.yaml
	kubectl delete -f deploy/webhook.yaml

