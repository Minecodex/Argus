"""Execute alert domain tests on an owned disposable database, never formal data."""
from manage import Run
from B13a.postgres_tests import main

if __name__ == '__main__':
    main(['./internal/alerting'])
