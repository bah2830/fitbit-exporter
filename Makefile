APP_NAME := fitbit-exporter

build_image:
	docker build -t $(APP_NAME) .

debug: build_image remove_containers
	$(eval RUN_FLAG = --rm)

deploy: build_image remove_containers

start_database:
	-docker-compose up -d db

stop_database:
	-docker-compose rm -sf db

start_containers:
	-docker-compose up -d

remove_containers:
	-docker-commpose rm -sf
