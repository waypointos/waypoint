#include "platform/Descriptor.hpp"

#include <set>
#include <sstream>

#include <tomlplusplus/toml.hpp>

namespace wp::platform {

namespace {
bool fail(std::string* err, const std::string& msg) {
    if (err) *err = "descriptor: " + msg;
    return false;
}

bool parseJoint(const toml::table& t, Joint* j, std::string* err) {
    auto name = t["name"].value<std::string>();
    auto driver = t["driver"].value<std::string>();
    auto busId = t["bus_id"].value<std::int64_t>();
    auto type = t["type"].value<std::string>();
    auto ownership = t["ownership"].value<std::string>();
    if (!name || !driver || !busId || !type || !ownership)
        return fail(err, "joint missing required field (name/driver/bus_id/type/ownership)");
    if (*busId < 1 || *busId > 253) return fail(err, "joint " + *name + " bus_id out of range");
    j->name = *name;
    j->driver = *driver;
    j->busId = static_cast<std::uint8_t>(*busId);
    j->type = *type;
    j->ownership = *ownership;
    j->invert = t["invert"].value_or(false);
    if (auto ci = t["command_interfaces"].as_array()) {
        for (auto&& v : *ci)
            if (auto s = v.value<std::string>()) j->commandInterfaces.push_back(*s);
    }
    if (auto lim = t["limits"].as_table()) {
        j->limits.velocityRadps = (*lim)["velocity_radps"].value<double>();
        j->limits.positionMinRad = (*lim)["position_min_rad"].value<double>();
        j->limits.positionMaxRad = (*lim)["position_max_rad"].value<double>();
    }
    return true;
}
}  // namespace

std::optional<Descriptor> Descriptor::parse(const std::string& tomlSrc, std::string* err) {
    toml::table root;
    try {
        root = toml::parse(tomlSrc);
    } catch (const toml::parse_error& e) {
        fail(err, std::string("parse error: ") + e.what());
        return std::nullopt;
    }

    Descriptor d;
    d.schema = static_cast<int>(root["schema"].value_or(std::int64_t{0}));
    if (d.schema != 1) {
        fail(err, "unsupported schema " + std::to_string(d.schema) + " (supported: 1)");
        return std::nullopt;
    }
    d.platformId = root["platform"]["id"].value_or(std::string{});
    d.platformName = root["platform"]["name"].value_or(std::string{});
    d.vehicleClass = root["platform"]["vehicle_class"].value_or(std::string{});
    if (d.platformId.empty()) { fail(err, "platform.id is required"); return std::nullopt; }
    if (d.vehicleClass != "diff_drive_rover" && d.vehicleClass != "fixed_base") {
        fail(err, "unknown vehicle_class \"" + d.vehicleClass + "\"");
        return std::nullopt;
    }

    if (auto drivers = root["drivers"].as_table()) {
        for (auto&& [key, val] : *drivers) {
            auto* t = val.as_table();
            if (!t) continue;
            DriverCfg cfg;
            cfg.kind = (*t)["kind"].value_or(std::string{});
            cfg.port = (*t)["port"].value_or(std::string{});
            cfg.baud = static_cast<int>((*t)["baud"].value_or(std::int64_t{1000000}));
            if (cfg.kind != "sts3215" && cfg.kind != "sim") {
                fail(err, "driver " + std::string(key.str()) + " has unknown kind \"" + cfg.kind + "\"");
                return std::nullopt;
            }
            if (cfg.kind == "sts3215" && cfg.port.empty()) {
                fail(err, "driver " + std::string(key.str()) + " (sts3215) requires port");
                return std::nullopt;
            }
            d.drivers[std::string(key.str())] = cfg;
        }
    }
    if (d.drivers.empty()) { fail(err, "[drivers] is required"); return std::nullopt; }

    std::set<std::string> names;
    std::set<std::pair<std::string, std::uint8_t>> busIds;
    if (auto joints = root["joints"].as_array()) {
        for (auto&& el : *joints) {
            auto* t = el.as_table();
            if (!t) { fail(err, "joints entries must be tables"); return std::nullopt; }
            Joint j;
            if (!parseJoint(*t, &j, err)) return std::nullopt;
            if (!names.insert(j.name).second) {
                fail(err, "duplicate joint name \"" + j.name + "\"");
                return std::nullopt;
            }
            if (d.drivers.find(j.driver) == d.drivers.end()) {
                fail(err, "joint \"" + j.name + "\" references undefined driver \"" + j.driver + "\"");
                return std::nullopt;
            }
            if (!busIds.insert({j.driver, j.busId}).second) {
                fail(err, "duplicate bus_id " + std::to_string(j.busId) + " on driver \"" + j.driver + "\"");
                return std::nullopt;
            }
            d.joints.push_back(std::move(j));
        }
    }
    if (d.joints.empty()) { fail(err, "[[joints]] is required"); return std::nullopt; }

    auto kin = root["kinematics"].as_table();
    if (d.vehicleClass == "fixed_base") {
        if (kin) {
            fail(err, "vehicle_class fixed_base must not declare [kinematics]");
            return std::nullopt;
        }
        return d;
    }
    if (!kin) { fail(err, "[kinematics] is required for diff_drive_rover"); return std::nullopt; }
    DescriptorKinematics k;
    k.model = (*kin)["model"].value_or(std::string{});
    k.wheelRadiusM = (*kin)["wheel_radius_m"].value_or(0.0);
    k.trackWidthM = (*kin)["track_width_m"].value_or(0.0);
    if (k.model != "diff_drive" || k.wheelRadiusM <= 0 || k.trackWidthM <= 0) {
        fail(err, "kinematics requires model=\"diff_drive\" and positive wheel_radius_m/track_width_m");
        return std::nullopt;
    }
    if (auto wheels = (*kin)["wheels"].as_table()) {
        for (auto&& [pos, val] : *wheels) {
            auto jointName = val.value<std::string>();
            if (!jointName) { fail(err, "kinematics.wheels values must be strings"); return std::nullopt; }
            k.wheels[std::string(pos.str())] = *jointName;
        }
    }
    d.kinematics = std::move(k);
    for (const char* pos : {"front_left", "front_right", "back_left", "back_right"}) {
        auto it = d.kinematics->wheels.find(pos);
        if (it == d.kinematics->wheels.end()) {
            fail(err, std::string("kinematics.wheels missing position \"") + pos + "\"");
            return std::nullopt;
        }
        const Joint* j = d.jointByName(it->second);
        if (!j || j->type != "wheel") {
            fail(err, "kinematics.wheels " + std::string(pos) + " must reference a declared wheel joint");
            return std::nullopt;
        }
    }
    return d;
}

std::optional<Descriptor> Descriptor::load(const std::string& path, std::string* err) {
    toml::table root;
    try {
        root = toml::parse_file(path);
    } catch (const std::exception& e) {
        fail(err, path + ": " + e.what());
        return std::nullopt;
    }
    std::stringstream ss;
    ss << root;
    return parse(ss.str(), err);
}

const Joint* Descriptor::jointByName(const std::string& name) const {
    for (const auto& j : joints)
        if (j.name == name) return &j;
    return nullptr;
}

std::vector<std::uint8_t> Descriptor::platformOwnedBusIds() const {
    std::vector<std::uint8_t> out;
    for (const auto& j : joints)
        if (j.ownership == "platform") out.push_back(j.busId);
    return out;
}

std::vector<std::uint8_t> Descriptor::moduleWheelBusIds() const {
    std::vector<std::uint8_t> out;
    for (const auto& j : joints)
        if (j.ownership == "module" && j.type == "wheel") out.push_back(j.busId);
    return out;
}

std::vector<std::uint8_t> Descriptor::allBusIds() const {
    std::vector<std::uint8_t> out;
    for (const auto& j : joints) out.push_back(j.busId);
    return out;
}

}  // namespace wp::platform
