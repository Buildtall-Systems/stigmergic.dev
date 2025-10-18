# Nostr Test Keys

This file contains test keypairs for use in tests involving Nostr entities.

## Generation Method

These keys were generated using the `nak` command-line tool:

1. Generate secret key (private key): `nak key generate`
2. Derive public key from secret: `nak key public <secret_key_hex>`
3. Encode public key as npub: `nak encode npub <public_key_hex>`

## Test Keypairs

| ID | Secret Key (hex) | Public Key (hex) | npub |
|----|------------------|------------------|------|
| 1 | 2b9383f48c3b125ced4d996346c2b245c0ad259642d0a3baeadfb436a02d3f70 | 08d706bf312f273ee38df304671b38a94c6ef2313100af8b92898224990f115c | npub1prtsd0e39unnacud7vzxwxec49xxau33xyq2lzuj3xpzfxg0z9wqjn0v8q |
| 2 | 31f5e65daeee391683f8974b1a7646fbb205946f7d75d714d9d0aad9f1bf39f8 | 1d681c05cb04261a2de191cde2168b867d68aebe086bd11ce5e838b77efe6e20 | npub1r45pcpwtqsnp5t0pj8x7y95tse7k3t47pp4az889aqutwlh7dcsql04zht |
| 3 | 4536ec92545cdb6a5849f6107deebd35a0aeb0f0005bfa630daf1e16ff1d3cca | 895b66afc35e0133b8665e769908fdbd7dfa63c9e18c0ba61908b0e6740e010b | npub139dkdt7rtcqn8wrxtemfjz8ah47l5c7fuxxqhfsepzcwvaqwqy9s34w8ju |
| 4 | 328eacf7f43fec05641e80d8ac47d45d3bf56d45e5e4666028270025e6f879ae | 980ba536a5fd9453d9d7276930b04d81fe972fc1c7f666d03fdc2a781cc48eac | npub1nq962d49lk298kwhya5npvzds8lfwt7pclmxd5plms48s8xy36kqkgt7st |
| 5 | 50fb38088ccc0a728c776a28542c7c0dda30fecb8d599b7fdbba85bd3e57cc4d | f578f735c1ef93eebb0a74fd54aa4a1bf0412092a630ac489fcee03dd7c6d236 | npub174u0wdwpa7f7awc2wn74f2j2r0cyzgyj5cc2cjylemsrm47x6gmqc2u72q |
| 6 | 781b17be85937c3ac64dc9f54c8cbf7ae1be3ebe708e13df055af8ef179aa854 | 394545797f41e8d0bd52622309ae5648b32ec703f1a11635e4d32b83faebac42 | npub189z527tlg85dp02jvg3sntjkfzeja3cr7xs3vd0y6v4c87ht43pqp77u68 |
| 7 | f4778bac327158212bf01578411c95de96b9b35573eebc171cd4151fc34d6ba6 | 620bda48765a4503dd005f300c98106c9a3452413be756052d1ca534b4e257fe | npub1vg9a5jrktfzs8hgqtucqexqsdjdrg5jp80n4vpfdrjjnfd8z2llqy96c4f |
| 8 | 746f0ce70a65de65c5bd5913fedcc241e3933fb51ea495f82312782507b9a594 | 75d1e5128f3be9e3f7c044e3978bc2c159d1aac21b799a7e71b72e2fefa4a1ad | npub1whg72y5080578a7qgn3e0z7zc9var2kzrdue5ln3kuhzlmay5xks6madky |
| 9 | ee7d172e803a23118c9cb5bb6f67c93b38d65a097531275d9271ab887f55c825 | 96aa2f07e0206ca2fbc237a636a99d998135af040dc61ab3320072fb870cb85e | npub1j64z7plqypk2977zx7nrd2vanxqnttcyphrp4vejqpe0hpcvhp0qr4war8 |
| 10 | 3aa60f9c5382c7a7b0959b892f6df16f386e11401af6e2fef2ec884b63b1741e | a10ef6af3a8f66729ecadc2fb877aa737242c14a53727d4d8d350af1e1fa95bd | npub15y80dte63an898k2mshmsaa2wdey9s222de86nvdx590rc06jk7swvvv2p |

## Usage

When writing tests that involve Nostr entities (npub, note, nevent, etc.), always use valid identifiers from this table. Never make up fake bech32 strings.

For note IDs, you can use the public key hex values and encode them as `note` using:
```bash
nak encode note <public_key_hex>
```
