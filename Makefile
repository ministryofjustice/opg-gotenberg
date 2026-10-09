
build:
	docker compose build opg-gotenberg

test-server-spec: setup-directories up run-goss down

up:
	docker compose up -d opg-gotenberg

down:
	docker compose down

run-goss:
	docker compose run --rm goss
	docker compose exec -T opg-gotenberg /goss-bin/goss --gossfile /goss-bin/goss.yaml validate --retry-timeout 30s --format junit > test-results/junit/opg-gotenberg-goss.xml

setup-directories:
	mkdir -p -m 0777 test-results/junit
