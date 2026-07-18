-- 083: Drop tenant_profiles.
--
-- Created in 001 for the "Tenant Profile" concept; no production or test code
-- ever read or wrote it and it holds zero rows. Tenant-level customization is
-- carried by Connector/Semantic Mapping/Capability/Policy configuration, not
-- this key-value table. (tenants itself stays: it anchors 12 FKs.)

DROP TABLE IF EXISTS tenant_profiles;
