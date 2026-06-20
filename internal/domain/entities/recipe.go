package entities

type Recipe struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Include     []Clause `toml:"include"`
	Exclude     []Clause `toml:"exclude"`
	Net         bool     `toml:"net"`
}

type Clause struct {
	Field string `toml:"field"`
	Op    string `toml:"op"`
	Value string `toml:"value"`
}
