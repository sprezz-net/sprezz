from logging import getLogger

from gino import GinoConnection

from .client import ClientService


log = getLogger(__name__)


class ConnectService:
    def __init__(self,
                 connection: GinoConnection,
                 client_service: ClientService) -> None:
        self.connection = connection
        self.client_service = client_service
