#include <gtest/gtest.h>

#include <memory>
#include <set>

#include "servo/Discovery.hpp"
#include "servo/MockUart.hpp"
#include "servo/Sts3215Bus.hpp"
#include "servo/Sts3215Frame.hpp"

using namespace wp::servo;

TEST(Discovery, ReturnsOnlyRespondingIds) {
    const std::set<std::uint8_t> present = {2, 3, 7};
    auto mock = std::make_unique<MockUart>([present](const std::vector<std::uint8_t>& req) {
        std::uint8_t id = req[2];
        if (present.count(id) == 0) return std::vector<std::uint8_t>{};  // absent → no reply
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(discoverServos(bus, 1, 8, 5), (std::vector<std::uint8_t>{2, 3, 7}));
}

TEST(Discovery, ByIdSetReturnsOnlyResponders) {
    const std::set<std::uint8_t> present = {3, 9};
    auto mock = std::make_unique<MockUart>([present](const std::vector<std::uint8_t>& req) {
        std::uint8_t id = req[2];
        if (present.count(id) == 0) return std::vector<std::uint8_t>{};  // absent → no reply
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(discoverServos(bus, std::vector<std::uint8_t>{3, 5, 9}, 5),
              (std::vector<std::uint8_t>{3, 9}));
}
