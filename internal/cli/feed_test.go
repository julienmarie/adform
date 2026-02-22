package cli

import "testing"

func TestParseXMLFeedExtractsProducts(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0"?>
<rss xmlns:g="http://base.google.com/ns/1.0">
  <channel>
    <item>
      <g:id>sku_1</g:id>
      <title>Duck Breast</title>
      <link>https://example.com/p/sku_1</link>
      <g:price>1499 PHP</g:price>
      <g:availability>in stock</g:availability>
    </item>
    <item>
      <g:id>sku_2</g:id>
      <title>Caviar</title>
      <link>https://example.com/p/sku_2</link>
      <g:price>2999 PHP</g:price>
      <g:availability>out of stock</g:availability>
    </item>
  </channel>
</rss>`)

	products, detected, err := parseFeedProducts(xmlData, "auto", "https://example.com/feed.xml", "application/xml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if detected != "xml" {
		t.Fatalf("expected xml detection, got %q", detected)
	}
	if len(products) != 2 {
		t.Fatalf("expected 2 products, got %d", len(products))
	}
	if products[0].ID != "sku_1" {
		t.Fatalf("expected sku_1, got %q", products[0].ID)
	}
	if products[0].Price != "1499 PHP" {
		t.Fatalf("expected price, got %q", products[0].Price)
	}
	if products[0].InStock == nil || !*products[0].InStock {
		t.Fatalf("expected first product in stock")
	}
	if products[1].InStock == nil || *products[1].InStock {
		t.Fatalf("expected second product out of stock")
	}
}

func TestParseCSVFeedExtractsProducts(t *testing.T) {
	csvData := []byte("id,title,link,price,availability\nsku_1,Duck Breast,https://example.com/p/sku_1,1499 PHP,in stock\nsku_2,Caviar,https://example.com/p/sku_2,2999 PHP,out of stock\n")
	products, detected, err := parseFeedProducts(csvData, "auto", "https://example.com/feed.csv", "text/csv")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if detected != "csv" {
		t.Fatalf("expected csv detection, got %q", detected)
	}
	if len(products) != 2 {
		t.Fatalf("expected 2 products, got %d", len(products))
	}
	if products[1].ID != "sku_2" {
		t.Fatalf("expected sku_2, got %q", products[1].ID)
	}
	if products[1].InStock == nil || *products[1].InStock {
		t.Fatalf("expected second product out of stock")
	}
}

func TestBuildValidateInStockParser(t *testing.T) {
	if v := parseInStock("in stock"); v == nil || !*v {
		t.Fatalf("expected in stock true")
	}
	if v := parseInStock("out of stock"); v == nil || *v {
		t.Fatalf("expected out of stock false")
	}
	if v := parseInStock("maybe"); v != nil {
		t.Fatalf("expected unknown availability to be nil")
	}
}
