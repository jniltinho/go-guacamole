-- Profile fields of the Java client's user form (also imported from guacamole_user).
ALTER TABLE users ADD COLUMN organization {{STR}}(256);
ALTER TABLE users ADD COLUMN organizational_role {{STR}}(256);
