package com.edgewatcher.di;

import com.edgewatcher.domain.port.IdGenerator;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_IdsFactory implements Factory<IdGenerator> {
  @Override
  public IdGenerator get() {
    return ids();
  }

  public static AppModule_IdsFactory create() {
    return InstanceHolder.INSTANCE;
  }

  public static IdGenerator ids() {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.ids());
  }

  private static final class InstanceHolder {
    static final AppModule_IdsFactory INSTANCE = new AppModule_IdsFactory();
  }
}
