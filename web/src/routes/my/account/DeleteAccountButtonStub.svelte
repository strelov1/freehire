<!-- Stands in for DeleteAccountButton in page.spec.ts. That component has its own spec and
     pulls in the billing client, the identity fetch and a dialog; none of it is what these
     cases are about, and `vi.mock` needs a real module to point at. -->
<span data-testid="delete-account-stub"></span>
