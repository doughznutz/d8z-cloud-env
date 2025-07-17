# Shortcuts into github, be sure docker.mk is included or it wont build.
SERVICES += github

.PHONY: create_repo
create_repo: build-github
	docker compose run github create_repo

.PHONY: create_public_repo
create_public_repo: build-github
	docker compose run github create_repo public

.PHONY: diff
diff: build-github
	docker compose run github diff_repo

.PHONY: branch
branch: build-github
	docker compose run github create_branch clean

.PHONY: rebase
rebase: build-github
	docker compose run github rebase_repo

.PHONY: rename_branch
rename_branch: build-github
	docker compose run github rename_branch $(OLD_NAME) $(NEW_NAME)

.PHONY: pull_request
pull_request:
	docker compose run github create_pull_request $(BRANCH)