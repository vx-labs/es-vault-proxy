build:: docker
	docker create --name artifacts quay.io/vxlabs/es-vault-proxy
	docker cp artifacts:/bin/es-vault-proxy quay.io/es-vault-proxy
	docker rm artifacts

docker::
	docker build -t quay.io/vxlabs/es-vault-proxy .

push:: docker
	docker push quay.io/vxlabs/es-vault-proxy
