Intro
=====
Various notes about the Sprezz code base.

ZODB
====
Notes about ZODB usage.

Volatile attributes in ZODB
---------------------------
ZODB is a persistent object datastore. Not all attributes of objects need to
be persistent though. Volatile attributes can be used to cache expensive
operations.::

  def operation(self):
      try:
          return self._v_data
      except AttributeError:
          data = expensive_operation()
          self._v_data = data
          return data

Use try except to avoid a race condition when the garbage collector decides to
remove the object from memory. Always store the result of an expensive
operation in a variable and return using that same variable. When you use the
volatile version it might get stuck in a race condition again.

One should assume that objects can be invalidated at any time, not just on
transaction boundaries. Please see the `Dangerous _v_ attributes
<http://www.upfrontsystems.co.za/Members/jean/zope-notes/dangerous_v_>`_
article for more information.

In general one should only store values in volatile attributes that involve
only one object.
