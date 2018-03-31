from datetime import datetime
from enum import Enum

from gino import Gino
from sqlalchemy.ext.hybrid import hybrid_property, hybrid_method


db = Gino()


class Account(db.Model):  # type: ignore
    __tablename__ = 'account'

    id = db.Column(db.BigInteger(), primary_key=True)
    username = db.Column(db.Unicode(), unique=True)
    email = db.Column(db.Unicode())
    email_verified = db.Column(db.Boolean(), default=False)
    disabled = db.Column(db.Boolean(), nullable=False,
                         default=False)
    created = db.Column(db.DateTime(), nullable=False,
                        default=datetime.utcnow)
    expires = db.Column(db.DateTime())
    changed = db.Column(db.DateTime(), onupdate=datetime.utcnow)
    password_hash = db.Column(db.Unicode())
    password_changed = db.Column(db.DateTime())
    password_attempts = db.Column(db.Integer(), default=0)
    last_attempt = db.Column(db.DateTime())
    last_login = db.Column(db.DateTime())

    @hybrid_property
    def expired(self) -> bool:
        if self.expires is None:
            return False
        if self.expires <= datetime.utcnow():
            return True
        return False

    @hybrid_method
    def password_expired(self, expire_date: datetime) -> bool:
        if self.password_hash is None:
            return True
        if self.password_changed is None:
            return False
        if self.password_changed <= expire_date:
            return True
        return False

    @hybrid_method
    def locked(self, threshold_count: int, threshold_time: datetime) -> bool:
        if self.password_attempts is None:
            return False
        if self.password_attempts < threshold_count:
            return False
        if self.last_attempt < threshold_time:
            self.password_attempts = 0
            return False
        return True

    def to_json(self):
        return {'username': self.username,
                'email': self.email}


class ClientType(Enum):
    CONFIDENTIAL = 'confidential'
    PUBLIC = 'public'


class GrantType(Enum):
    AUTHORIZATION_CODE = 'authorization_code'
    IMPLICIT = 'implicit'
    PASSWORD = 'password'
    CLIENT_CREDENTIALS = 'client_credentials'


class Client(db.Model):  # type: ignore
    __tablename__ = 'client'

    id = db.Column(db.Unicode(), primary_key=True)
    name = db.Column(db.Unicode())
    # TODO Hash the secret?
    secret = db.Column(db.Unicode(), nullable=False, unique=True)
    client_type = db.Column(db.Enum(ClientType), nullable=False,
                            default=ClientType.CONFIDENTIAL)
    grant_type = db.Column(db.Enum(GrantType), nullable=False,
                           default=GrantType.AUTHORIZATION_CODE)
    owner_id = db.Column(db.ForeignKey('account.id', ondelete='CASCADE'))
    disabled = db.Column(db.Boolean(), nullable=False, default=False)
    created = db.Column(db.DateTime(), nullable=False,
                        default=datetime.utcnow)
    expires = db.Column(db.DateTime())
    changed = db.Column(db.DateTime(), onupdate=datetime.utcnow)

    @hybrid_property
    def expired(self) -> bool:
        if self.expires is None:
            return False
        if self.expires <= datetime.utcnow():
            return True
        return False

    @hybrid_property
    def valid(self) -> bool:
        if self.disabled:
            return False
        if self.expired:
            return False
        return True

    def to_json(self):
        return {'name': self.name,
                'client_id': self.id,
                'client_secret': self.secret,  # TODO Debug only
                'client_type': self.client_type.value,
                'grant_type': self.grant_type.value,
                'owner_id': self.owner_id}


class ClientRedirect(db.Model):  # type: ignore
    __tablename__ = 'client_redirect'

    id = db.Column(db.BigInteger(), primary_key=True)
    client_id = db.Column(db.ForeignKey('client.id', ondelete='CASCADE'))
    redirect_uri = db.Column(db.Unicode(), nullable=False)
    default = db.Column(db.Boolean(), nullable=False, default=False)

    def to_json(self):
        return {'redirect_uri': self.redirect_uri,
                'default': self.default}
