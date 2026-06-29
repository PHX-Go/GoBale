# Diagnostic & Validation Utilities

GoBale includes a complete set of diagnostic and validation tools optimized for validating business logic, formatting system metrics, and preventing cryptographic timing attacks.

---

## National Code Validator (`ValidateNationalCode`)

Verifies the cryptographic checksum of Iranian National Codes (کد ملی) based on the official check-digit algorithm.

* **Fake Input Filtration:** It explicitly rejects sequential mock inputs with completely identical repetitive digits (e.g., `"1111111111"`), which standard mathematical checksums might otherwise evaluate as valid.

```go
bot.On().Cmd("verifycode").Do(func(c *gobale.Ctx) {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return
	}
	code := args[0]

	// Cryptographically validate the input national code
	if gobale.ValidateNationalCode(code) {
		_, _ = c.Send().Text("✅ The National Code is valid.").Go()
		return
	}

	_, _ = c.Send().Text("❌ The National Code is invalid!").Go()
})
```

---

## Phone Number Normalizer (`NormalizePhone` & `NormalizeSafirPhone`)

Sanitizes and normalizes Iranian mobile numbers by removing country prefixes (`+98`, `0098`, `98`), dashes, and whitespace, formatting them into standard patterns.

* **`NormalizePhone`**: Formats any valid Iranian mobile number to the standard `09xxxxxxxxx` representation.
* **`NormalizeSafirPhone`**: Formats any valid Iranian mobile number to the strict Safir-compliant `989xxxxxxxxx` representation.

```go
bot.On().Cmd("verifyphone").Do(func(c *gobale.Ctx) {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return
	}
	rawInput := args[0]

	// Normalize phone to standard format (e.g., 09123456789)
	normalized, ok := gobale.NormalizePhone(rawInput)
	if ok {
		_, _ = c.Send().Text("Standard Format: " + normalized).Go()
	}

	// Normalize phone to Safir format (e.g., 989123456789)
	safirFormat, ok := gobale.NormalizeSafirPhone(rawInput)
	if ok {
		_, _ = c.Send().Text("Safir Format: " + safirFormat).Go()
	}
})
```

---

## Memory Byte Formatter (`FormatBytes`)

Converts raw file sizes or memory heap allocations represented in bytes into human-readable metric strings (e.g., KB, MB, GB).

```go
bot.On().Cmd("storage").Do(func(c *gobale.Ctx) {
	// Format memory allocated bytes into readable metric string
	rawBytes := int64(104857600) // 100 MB represented in bytes
	readableMetric := gobale.FormatBytes(rawBytes)

	_, _ = c.Send().
		Text("Database size: " + readableMetric). // Returns "100.00 MB"
		Go()
})
```

---

## Constant-Time Secure Comparer (`SecureCompare`)

Compares two strings in constant-time. This mitigates cryptographic timing attack risks where attackers attempt to guess sensitive keys, webapp hashes, or passwords by measuring processing time differences.

```go
bot.On().Cmd("checkauth").Do(func(c *gobale.Ctx) {
	args, ok := c.Arg().([]string)
	if !ok || len(args) < 2 {
		return
	}
	inputHash := args[0]
	expectedHash := args[1]

	// Securely compare sensitive hashes in constant time
	if gobale.SecureCompare(inputHash, expectedHash) {
		_, _ = c.Send().Text("✅ Authentication successful!").Go()
		return
	}

	_, _ = c.Send().Text("❌ Authentication failed!").Go()
})
```
