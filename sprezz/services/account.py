from datetime import datetime

from gino import GinoConnection
from passlib.context import CryptContext

from sprezz.models import Account
from sprezz.exceptions import (AccountDisabled,
                               AccountExpired,
                               AccountLocked,
                               PasswordExpired,
                               InvalidAccountOrPassword)


__all__ = (
    'AccountService',
)


PWD_CONTEXT = CryptContext(schemes=['bcrypt', 'pbkdf2_sha512'],
                           deprecated='auto')


class AccountService:
    def __init__(self,
                 connection: GinoConnection,
                 account: Account = None) -> None:
        self.connection = connection
        self.account = account

    async def get_account(self, username: str) -> Account:
        self.account = await self.connection.first(
            Account.query.where(Account.username == username))
        return self.account

    def _set_password(self, password: str)-> None:
        self.account.password_hash = PWD_CONTEXT.hash(password)

    def _verify_password(self, password: str)-> bool:
        if PWD_CONTEXT.verify(password, self.account.password_hash):
            if PWD_CONTEXT.needs_update(self.account.password_hash):
                self._set_password(password)
            return True
        return False

    async def create_account(self, username: str, email: str,
                             password: str = None) -> Account:
        if password is None:
            disabled = True
            password_hash = None
        else:
            password_hash = PWD_CONTEXT.hash(password)
            disabled = False
        self.account = await Account.create(bind=self.connection,
                                            username=username,
                                            email=email,
                                            email_verified=False,
                                            disabled=disabled,
                                            password_hash=password_hash)
        return self.account

    async def change_password(self,
                              old_password: str, new_password: str,
                              threshold_count: int,
                              threshold_time: datetime) -> bool:
        if self.account.disabled:
            raise AccountDisabled('Account is disabled')
        elif self.account.expired:
            raise AccountExpired('Account is expired')
        elif self.account.locked(threshold_count, threshold_time):
            raise AccountLocked('Account is locked')
        elif self._verify_password(old_password):
            self._set_password(new_password)
            self.account.password_changed = datetime.utcnow()
            return True
        else:
            raise InvalidAccountOrPassword('Invalid account and/or password')

    async def attempt_password(self, password: str) -> bool:
        self.account.last_attempt = datetime.utcnow()
        if self._verify_password(password):
            self.account.password_attempts = None
            return True
        self.account.password_attempts += 1
        return False

    async def login(self,
                    password: str,
                    threshold_count: int,
                    threshold_time: datetime,
                    expire_date: datetime) -> bool:
        if self.account.disabled:
            raise AccountDisabled('Account is disabled')
        elif self.account.expired:
            raise AccountExpired('Account is expired')
        elif self.account.locked(threshold_count, threshold_time):
            raise AccountLocked('Account is locked')
        elif self.account.password_expired(expire_date):
            raise PasswordExpired('Password has expired')
        elif self.attempt_password(password):
            self.account.last_login = self.account.last_attempt
            return True
        else:
            raise InvalidAccountOrPassword('Invalid account and/or password')

    async def query_all_accounts(self) -> Account:
        return await self.connection.all(Account.query)

    def iterate_all_accounts(self):
        return self.connection.iterate(Account.query)
