class SprezzException(Exception):
    def __init__(self, msg):
        super(SprezzException, self).__init__(msg)
        self.msg = msg


class AccountDisabled(SprezzException):
    def __init__(self, msg):
        super(AccountDisabled, self).__init__(msg)
        self.msg = msg


class AccountExpired(SprezzException):
    def __init__(self, msg):
        super(AccountExpired, self).__init__(msg)
        self.msg = msg


class AccountLocked(SprezzException):
    def __init__(self, msg):
        super(AccountLocked, self).__init__(msg)
        self.msg = msg


class PasswordExpired(SprezzException):
    def __init__(self, msg):
        super(PasswordExpired, self).__init__(msg)
        self.msg = msg


class InvalidAccountOrPassword(SprezzException):
    def __init__(self, msg):
        super(InvalidAccountOrPassword, self).__init__(msg)
        self.msg = msg
