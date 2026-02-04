# Work In Progress Examples

This directory contains resource examples that are not yet ready for conformance testing.

## Files

### gke-cluster.pkl

GKE cluster example. Excluded due to long provisioning times and resource costs.

### sql-database-instance.pkl

Cloud SQL instance example. Excluded due to:

1. **Organization policy constraint:** The project has `constraints/sql.restrictPublicIp` enabled, blocking public IPs
2. **Private IP requires VPC setup:** Would need Private Service Connection to a VPC network
3. **Long provisioning times:** ~10-15 minutes per instance
4. **Resource costs:** Cloud SQL instances incur charges

**Error:** `Organization Policy check failure: constraints/sql.restrictPublicIp enforced`

**To enable:** Either:
- Create a VPC network with Private Service Connection and configure `privateNetwork` in `ipConfiguration`
- Or request exception to the organization policy for the test project

### gcp_lifeline.pkl

Experimental lifeline configuration example.
