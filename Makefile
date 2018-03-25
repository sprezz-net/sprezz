install:
	pip install --upgrade .

develop:
	pip install --upgrade -e .

lint:
	pylint sprezz; exit $$(($$? & 35))
	mypy --ignore-missing-imports sprezz
	pycodestyle sprezz
